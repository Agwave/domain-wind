// Package avail 探测域名是否已被注册（RDAP + DNS NS，粗检）。
package avail

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Status 占用状态。
type Status string

const (
	// StatusTaken 已注册 / 已被占用。
	StatusTaken Status = "taken"
	// StatusLikelyFree 登记处未找到且无 NS，可能可注册（非下单保证）。
	StatusLikelyFree Status = "likely_free"
	// StatusUnknown 无法判断。
	StatusUnknown Status = "unknown"
)

// Result 单域名探测结果。
type Result struct {
	Domain string
	Status Status
	Detail string
	Err    string
}

type rdapOutcome int

const (
	rdapTaken rdapOutcome = iota
	rdapNotFound
	rdapError
)

// Client 可用性探测客户端。
type Client struct {
	http     *http.Client
	resolver *net.Resolver
	rdapBase string // 默认 https://rdap.org/domain ；单测可改
	skipDNS  bool
	conc     int
}

// New 创建客户端。
func New() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 12 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 8 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		resolver: net.DefaultResolver,
		rdapBase: "https://rdap.org/domain",
		conc:     5,
	}
}

// SetConcurrency 设置 CheckAll 并发度。
func (c *Client) SetConcurrency(n int) {
	if n > 0 {
		c.conc = n
	}
}

// NormalizeDomain 规范化输入为 host 形式（小写、去协议/路径/www）。
func NormalizeDomain(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return "", fmt.Errorf("空域名")
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("无效 URL: %s", raw)
		}
		s = u.Host
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	s = strings.TrimPrefix(s, "www.")
	s = strings.Trim(s, ".")
	if s == "" || !strings.Contains(s, ".") {
		return "", fmt.Errorf("需要包含 TLD 的域名: %s", raw)
	}
	labels := strings.Split(s, ".")
	for _, lab := range labels {
		if lab == "" {
			return "", fmt.Errorf("无效域名: %s", raw)
		}
	}
	return s, nil
}

func decide(rdap rdapOutcome, hasNS, dnsErr bool) Status {
	if rdap == rdapTaken || hasNS {
		return StatusTaken
	}
	if rdap == rdapNotFound && !dnsErr && !hasNS {
		return StatusLikelyFree
	}
	return StatusUnknown
}

// Check 探测单个域名。
func (c *Client) Check(ctx context.Context, raw string) Result {
	domain, err := NormalizeDomain(raw)
	if err != nil {
		return Result{Domain: raw, Status: StatusUnknown, Err: err.Error()}
	}
	res := Result{Domain: domain}

	var (
		hasNS, dnsErr bool
		details       []string
	)

	rdapOut, rdapDetail := c.lookupRDAP(ctx, domain)
	if rdapDetail != "" {
		details = append(details, rdapDetail)
	}

	if !c.skipDNS {
		var dnsDetail string
		hasNS, dnsErr, dnsDetail = c.lookupNS(ctx, domain)
		if dnsDetail != "" {
			details = append(details, dnsDetail)
		}
	}
	res.Status = decide(rdapOut, hasNS, dnsErr)
	res.Detail = strings.Join(details, "; ")
	return res
}

func (c *Client) lookupRDAP(ctx context.Context, domain string) (out rdapOutcome, detail string) {
	base := strings.TrimRight(c.rdapBase, "/")
	u := base + "/" + url.PathEscape(domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return rdapError, "rdap: " + err.Error()
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	req.Header.Set("User-Agent", "domain-wind/1.0")
	resp, err := c.http.Do(req)
	if err != nil {
		return rdapError, "rdap: " + err.Error()
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusOK:
		return rdapTaken, "rdap=200"
	case http.StatusNotFound:
		return rdapNotFound, "rdap=404"
	default:
		return rdapError, fmt.Sprintf("rdap=%d", resp.StatusCode)
	}
}

func (c *Client) lookupNS(ctx context.Context, domain string) (hasNS, dnsErr bool, detail string) {
	resolver := c.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ns, err := resolver.LookupNS(ctx, domain)
	if err != nil {
		// NXDOMAIN / no such host → 无 NS
		msg := err.Error()
		if strings.Contains(msg, "no such host") || strings.Contains(msg, "NXDOMAIN") || strings.Contains(msg, "server misbehaving") {
			// misbehaving 偏未知；no such host 当无 NS
			if strings.Contains(msg, "server misbehaving") {
				return false, true, "dns=" + msg
			}
			return false, false, "dns=no-ns"
		}
		return false, true, "dns=" + msg
	}
	if len(ns) > 0 {
		return true, false, fmt.Sprintf("dns=ns:%d", len(ns))
	}
	return false, false, "dns=no-ns"
}

// CheckAll 并发探测；conc<=0 时用默认 5。
func (c *Client) CheckAll(ctx context.Context, domains []string) []Result {
	conc := c.conc
	if conc <= 0 {
		conc = 5
	}
	type job struct {
		i int
		d string
	}
	out := make([]Result, len(domains))
	ch := make(chan job)
	var wg sync.WaitGroup
	for n := 0; n < conc; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				out[j.i] = c.Check(ctx, j.d)
			}
		}()
	}
	for i, d := range domains {
		ch <- job{i: i, d: d}
	}
	close(ch)
	wg.Wait()
	return out
}

// StatusLabel 中文展示。
func StatusLabel(s Status) string {
	switch s {
	case StatusTaken:
		return "已注册"
	case StatusLikelyFree:
		return "可能可注册"
	default:
		return "未知"
	}
}

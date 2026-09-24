// Package avail 探测域名是否已被注册（RDAP + DNS NS，粗检）。
package avail

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
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

// 批检默认参数（公共 RDAP 易 429/超时，默认偏保守）。
const (
	DefaultConcurrency = 3
	DefaultMaxRetries  = 2 // 额外重试次数（总尝试 = 1 + MaxRetries）
	DefaultMinInterval = 200 * time.Millisecond
	DefaultBackoffBase = 400 * time.Millisecond
	maxRetryAfter      = 10 * time.Second
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

	maxRetries  int
	minInterval time.Duration
	backoffBase time.Duration

	mu       sync.Mutex
	lastRDAP time.Time
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
		resolver:    net.DefaultResolver,
		rdapBase:    "https://rdap.org/domain",
		conc:        DefaultConcurrency,
		maxRetries:  DefaultMaxRetries,
		minInterval: DefaultMinInterval,
		backoffBase: DefaultBackoffBase,
	}
}

// SetConcurrency 设置 CheckAll 并发度。
func (c *Client) SetConcurrency(n int) {
	if n > 0 {
		c.conc = n
	}
}

// SetMaxRetries 设置 RDAP 可恢复错误的额外重试次数（0=不重试）。
func (c *Client) SetMaxRetries(n int) {
	if n >= 0 {
		c.maxRetries = n
	}
}

// SetMinInterval 设置两次 RDAP 请求之间的最小间隔（<=0 关闭）。
func (c *Client) SetMinInterval(d time.Duration) {
	c.minInterval = d
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
	maxAttempts := 1 + c.maxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var (
		lastDetail        string
		pendingRetryAfter time.Duration
	)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			wait := c.retryWait(attempt, pendingRetryAfter)
			if err := sleepCtx(ctx, wait); err != nil {
				return rdapError, "rdap: " + err.Error()
			}
		}
		if err := c.throttle(ctx); err != nil {
			return rdapError, "rdap: " + err.Error()
		}

		out, detail, retryAfter, retryable := c.rdapOnce(ctx, domain)
		lastDetail = detail
		if !retryable {
			return out, detail
		}
		pendingRetryAfter = retryAfter
	}
	return rdapError, lastDetail
}

func (c *Client) rdapOnce(ctx context.Context, domain string) (out rdapOutcome, detail string, retryAfter time.Duration, retryable bool) {
	base := strings.TrimRight(c.rdapBase, "/")
	u := base + "/" + url.PathEscape(domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return rdapError, "rdap: " + err.Error(), 0, false
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	req.Header.Set("User-Agent", "domain-wind/1.0")
	resp, err := c.http.Do(req)
	if err != nil {
		return rdapError, "rdap: " + err.Error(), 0, isRetryableNetErr(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusOK:
		return rdapTaken, "rdap=200", 0, false
	case http.StatusNotFound:
		return rdapNotFound, "rdap=404", 0, false
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		ra := parseRetryAfter(resp.Header.Get("Retry-After"))
		return rdapError, fmt.Sprintf("rdap=%d", resp.StatusCode), ra, true
	default:
		return rdapError, fmt.Sprintf("rdap=%d", resp.StatusCode), 0, false
	}
}

func (c *Client) throttle(ctx context.Context) error {
	if c.minInterval <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	wait := c.minInterval - time.Since(c.lastRDAP)
	if wait > 0 {
		if err := sleepCtx(ctx, wait); err != nil {
			return err
		}
	}
	c.lastRDAP = time.Now()
	return nil
}

func (c *Client) retryWait(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return c.capRetryAfter(retryAfter)
	}
	base := c.backoffBase
	if base <= 0 {
		return 0
	}
	// attempt 从 1 起：400ms, 800ms, …
	shift := attempt - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 4 {
		shift = 4
	}
	d := base * time.Duration(1<<shift)
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	// 最多约 25% 抖动
	jitter := time.Duration(rand.Int64N(int64(d/4) + 1))
	return d + jitter
}

func (c *Client) capRetryAfter(d time.Duration) time.Duration {
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	if d < 0 {
		return 0
	}
	return d
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if sec, err := strconv.Atoi(v); err == nil {
		if sec < 0 {
			return 0
		}
		return time.Duration(sec) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

func isRetryableNetErr(err error) bool {
	if err == nil {
		return false
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, frag := range []string{
		"timeout",
		"temporary",
		"connection reset",
		"connection refused",
		"i/o timeout",
		"tls handshake timeout",
		"eof",
		"broken pipe",
	} {
		if strings.Contains(msg, frag) {
			return true
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) lookupNS(ctx context.Context, domain string) (hasNS, dnsErr bool, detail string) {
	resolver := c.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ns, err := resolver.LookupNS(ctx, domain)
	if err != nil {
		// 超时类再试一次
		if isRetryableNetErr(err) {
			if sleepErr := sleepCtx(ctx, 200*time.Millisecond); sleepErr == nil {
				ns, err = resolver.LookupNS(ctx, domain)
			}
		}
	}
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

// CheckAll 并发探测；conc<=0 时用 DefaultConcurrency。
func (c *Client) CheckAll(ctx context.Context, domains []string) []Result {
	conc := c.conc
	if conc <= 0 {
		conc = DefaultConcurrency
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

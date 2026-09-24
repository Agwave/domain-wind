package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const defaultGitHubTrendingURL = "https://github.com/trending?since=daily"

// GitHub 趋势页体积大、链路易抖：比 RSS 更积极重试（测试可覆盖这些变量）。
var (
	trendingRetries     = 5
	trendingTimeout     = 60 * time.Second
	trendingBackoffBase = 2 * time.Second
	trendingBackoffCap  = 16 * time.Second
)

var (
	trendingArticleRe = regexp.MustCompile(`(?is)<article\s+class="[^"]*\bBox-row\b[^"]*"[^>]*>(.*?)</article>`)
	trendingH2HrefRe  = regexp.MustCompile(`(?is)<h2\s+class="[^"]*\bh3\b[^"]*"[^>]*>.*?<a[^>]+href="(/[^"/]+/[^"/]+)"`)
	trendingDescRe    = regexp.MustCompile(`(?is)<p\s+class="[^"]*\bcolor-fg-muted\b[^"]*"[^>]*>(.*?)</p>`)
)

// FetchGitHubTrending 抓取 GitHub 趋势榜 HTML 并解析为条目。
// 对网络抖动、EOF、429/5xx 等做指数退避重试。
func (c *Client) FetchGitHubTrending(ctx context.Context, pageURL string) ([]Item, error) {
	if strings.TrimSpace(pageURL) == "" {
		pageURL = defaultGitHubTrendingURL
	}
	httpClient := c.trendingHTTP()
	var lastErr error
	for attempt := 0; attempt <= trendingRetries; attempt++ {
		if attempt > 0 {
			wait := trendingBackoff(attempt)
			if err := sleepCtx(ctx, wait); err != nil {
				return nil, err
			}
		}
		items, err := c.fetchTrendingOnce(ctx, httpClient, pageURL)
		if err == nil {
			return items, nil
		}
		lastErr = err
		if !trendingRetryable(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("重试 %d 次后仍失败: %w", trendingRetries, lastErr)
}

func (c *Client) trendingHTTP() *http.Client {
	timeout := trendingTimeout
	if c.http != nil && c.http.Timeout > timeout {
		timeout = c.http.Timeout
	}
	// 独立超时，避免与 RSS 共用 30s 导致大页读到一半 EOF。
	return &http.Client{Timeout: timeout}
}

func (c *Client) fetchTrendingOnce(ctx context.Context, httpClient *http.Client, pageURL string) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	// 接近常见浏览器，降低 GitHub 提前断连概率。
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return ParseTrendingHTML(body)
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusForbidden:
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	default:
		return nil, &trendingPermanentError{msg: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
}

// ParseTrendingHTML 解析 github.com/trending 页面中的仓库列表。
// 无 article.Box-row 时返回错误，避免把空页当成成功。
func ParseTrendingHTML(data []byte) ([]Item, error) {
	articles := trendingArticleRe.FindAllSubmatch(data, -1)
	if len(articles) == 0 {
		return nil, fmt.Errorf("未找到趋势仓库（页面结构可能已变更）")
	}
	pub := time.Now().UTC().Truncate(24 * time.Hour)
	var items []Item
	for _, m := range articles {
		body := m[1]
		hrefMatch := trendingH2HrefRe.FindSubmatch(body)
		if hrefMatch == nil {
			continue
		}
		href := string(hrefMatch[1])
		ownerRepo := strings.TrimPrefix(href, "/")
		if !isOwnerRepoPath(ownerRepo) {
			continue
		}
		summary := ""
		if dm := trendingDescRe.FindSubmatch(body); dm != nil {
			summary = truncate(cleanText(string(dm[1])), 500)
		}
		link := "https://github.com/" + ownerRepo
		items = append(items, Item{
			ID:        link,
			Title:     ownerRepo,
			Link:      link,
			Published: pub,
			Summary:   summary,
		})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("未找到趋势仓库（页面结构可能已变更）")
	}
	return dedupeItems(items), nil
}

func isOwnerRepoPath(path string) bool {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if p == "" || strings.ContainsAny(p, " ?#") {
			return false
		}
	}
	return true
}

type trendingPermanentError struct {
	msg string
}

func (e *trendingPermanentError) Error() string { return e.msg }

func trendingRetryable(err error) bool {
	if err == nil {
		return false
	}
	var perm *trendingPermanentError
	if errors.As(err, &perm) {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
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
		"http 429",
		"http 502",
		"http 503",
		"http 504",
		"http 403",
		"未找到趋势仓库", // 可能是挑战页/截断页，值得再试
		"请求失败",
		"读取响应",
	} {
		if strings.Contains(msg, frag) {
			return true
		}
	}
	return true
}

func trendingBackoff(attempt int) time.Duration {
	// attempt 从 1 起：2s, 4s, 8s, 16s…
	d := trendingBackoffBase
	for i := 1; i < attempt; i++ {
		if d >= trendingBackoffCap {
			return trendingBackoffCap
		}
		d *= 2
	}
	if d > trendingBackoffCap {
		return trendingBackoffCap
	}
	return d
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

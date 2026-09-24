// Package fetch 抓取并解析域名投资相关 RSS 源。
package fetch

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Item 一条 RSS 条目。
type Item struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Link      string    `json:"link"`
	Published time.Time `json:"published"`
	Summary   string    `json:"summary"`
}

// DedupKey 条目去重键：source+link，缺 link 则 source+title。
func DedupKey(source string, it *Item) string {
	if it == nil {
		return source + "|"
	}
	if it.Link != "" {
		return source + "|" + it.Link
	}
	return source + "|" + it.Title
}

// Client RSS 抓取客户端。
type Client struct {
	http    *http.Client
	retries int
}

// New 创建客户端；默认超时 30s，失败重试 2 次。
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		retries: 2,
	}
}

// FetchSource 抓取并解析一个 RSS URL，返回条目列表。
func (c *Client) FetchSource(ctx context.Context, url string) ([]Item, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		items, err := c.fetchOnce(ctx, url)
		if err == nil {
			return items, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (c *Client) fetchOnce(ctx context.Context, url string) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; domain-wind/1.0; +https://github.com/chenyinbo/domain-wind)")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应: %w", err)
	}
	return ParseFeed(body)
}

// ParseFeed 解析 RSS 2.0 或 Atom feed。
func ParseFeed(data []byte) ([]Item, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, fmt.Errorf("空响应")
	}
	// 去掉 XML 声明前的 BOM
	data = []byte(strings.TrimPrefix(trimmed, "\ufeff"))

	var rss rssFeed
	if err := xml.Unmarshal(data, &rss); err == nil && (len(rss.Channel.Items) > 0 || rss.Channel.Title != "") {
		return convertRSS(rss.Channel.Items), nil
	}
	var atom atomFeed
	if err := xml.Unmarshal(data, &atom); err == nil && len(atom.Entries) > 0 {
		return convertAtom(atom.Entries), nil
	}
	// 再试一次：有些 RSS 根元素带命名空间仍能用 Channel
	if len(rss.Channel.Items) > 0 {
		return convertRSS(rss.Channel.Items), nil
	}
	return nil, fmt.Errorf("无法解析为 RSS/Atom")
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Content     string `xml:"encoded"` // content:encoded
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Summary   string     `xml:"summary"`
	Content   atomText   `xml:"content"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
	Links     []atomLink `xml:"link"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func convertRSS(items []rssItem) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		title := cleanText(it.Title)
		link := strings.TrimSpace(it.Link)
		id := strings.TrimSpace(it.GUID)
		if id == "" {
			id = link
		}
		if id == "" {
			id = title
		}
		summary := cleanText(it.Description)
		if summary == "" {
			summary = cleanText(it.Content)
		}
		out = append(out, Item{
			ID:        id,
			Title:     title,
			Link:      link,
			Published: parseTime(it.PubDate),
			Summary:   truncate(summary, 500),
		})
	}
	return dedupeItems(out)
}

func convertAtom(entries []atomEntry) []Item {
	out := make([]Item, 0, len(entries))
	for _, e := range entries {
		title := cleanText(e.Title)
		link := atomPrimaryLink(e.Links)
		id := strings.TrimSpace(e.ID)
		if id == "" {
			id = link
		}
		if id == "" {
			id = title
		}
		summary := cleanText(e.Summary)
		if summary == "" {
			summary = cleanText(e.Content.Body)
		}
		pub := e.Published
		if pub == "" {
			pub = e.Updated
		}
		out = append(out, Item{
			ID:        id,
			Title:     title,
			Link:      link,
			Published: parseTime(pub),
			Summary:   truncate(summary, 500),
		})
	}
	return dedupeItems(out)
}

func atomPrimaryLink(links []atomLink) string {
	var alt, first string
	for _, l := range links {
		href := strings.TrimSpace(l.Href)
		if href == "" {
			continue
		}
		if first == "" {
			first = href
		}
		if l.Rel == "" || l.Rel == "alternate" {
			alt = href
			break
		}
	}
	if alt != "" {
		return alt
	}
	return first
}

func dedupeItems(items []Item) []Item {
	seen := map[string]bool{}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		key := it.Link
		if key == "" {
			key = it.Title
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, it)
	}
	return out
}

var (
	tagRe    = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe  = regexp.MustCompile(`\s+`)
	entityRe = regexp.MustCompile(`&\w+;`)
)

func cleanText(s string) string {
	s = html.UnescapeString(s)
	s = tagRe.ReplaceAllString(s, " ")
	s = entityRe.ReplaceAllStringFunc(s, html.UnescapeString)
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		time.RFC3339Nano,
		"Mon, 02 Jan 2006 15:04:05 MST",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// SalesSource 预留：二期接入 NameBio 等成交数据源。
type SalesSource interface {
	Name() string
	FetchSales(ctx context.Context, date string) ([]Sale, error)
}

// Sale 一笔域名成交（预留结构）。
type Sale struct {
	Domain string
	Price  int
	Date   string
	Venue  string
}

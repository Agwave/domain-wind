// Package config 加载配置文件：config.yaml（模板）+ config.local.yaml（本地覆盖）。
package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SourceTypeRSS 与空 Type 均表示 RSS/Atom。
const (
	SourceTypeRSS            = "rss"
	SourceTypeGitHubTrending = "github_trending"
)

// DefaultGitHubTrendingURL 每日、不限语言的 GitHub 趋势榜。
const DefaultGitHubTrendingURL = "https://github.com/trending?since=daily"

// Source 单个数据源（RSS 或 github_trending）。
type Source struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	Type    string `yaml:"type"` // ""|"rss"| "github_trending"
}

// IsGitHubTrending 是否为 GitHub 趋势榜源。
func (s Source) IsGitHubTrending() bool {
	return strings.EqualFold(strings.TrimSpace(s.Type), SourceTypeGitHubTrending)
}

// Analyze 热词分析参数。
type Analyze struct {
	BaselineDays   int      `yaml:"baseline_days"`
	NewMin         int      `yaml:"new_min"`
	SurgeMin       int      `yaml:"surge_min"`
	SurgeRatio     float64  `yaml:"surge_ratio"`
	TopK           int      `yaml:"top_k"`
	CommunityLimit int      `yaml:"community_limit"`
	MinWordLen     int      `yaml:"min_word_len"`
	MaxWordLen     int      `yaml:"max_word_len"`
	Stopwords      []string `yaml:"stopwords"`
}

// Notify 通知配置。
type Notify struct {
	WebhookURL      string `yaml:"webhook_url"`
	NotifyWhenQuiet bool   `yaml:"notify_when_quiet"`
}

// Candidates 热词 → 候选域名 → 占用粗检。
type Candidates struct {
	Enabled           bool     `yaml:"enabled"`
	TLDs              []string `yaml:"tlds"`
	MaxPerWord        int      `yaml:"max_per_word"`
	MinHotCount       int      `yaml:"min_hot_count"`
	Prefixes          []string `yaml:"prefixes"`
	Suffixes          []string `yaml:"suffixes"`
	CheckConcurrency  int      `yaml:"check_concurrency"`
	ReportFreeLimit   int      `yaml:"report_free_limit"`
	ReportTakenSample int      `yaml:"report_taken_sample"`
	MaxTotal          int      `yaml:"max_total"`
}

// Config 总配置。
type Config struct {
	Sources    map[string]Source `yaml:"sources"`
	Analyze    Analyze           `yaml:"analyze"`
	Notify     Notify            `yaml:"notify"`
	Candidates Candidates        `yaml:"candidates"`
}

// DefaultSourceOrder 报告与抓取的默认源顺序。
var DefaultSourceOrder = []string{"dnjournal", "dnwire", "namepros", "github_trending"}

// NewsSources 资讯类源（条目仍参与热词；报告不再单独列「今日资讯」）。
var NewsSources = map[string]bool{"dnjournal": true, "dnwire": true}

// CommunitySources 计入「社区讨论」的源。
var CommunitySources = map[string]bool{"namepros": true}

// Load 按顺序加载多个 YAML 配置文件，后者覆盖前者的同名键，并填充默认值。
func Load(paths ...string) (*Config, error) {
	cfg := &Config{Sources: make(map[string]Source)}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("读取配置 %s: %w", p, err)
		}
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("解析配置 %s: %w", p, err)
		}
	}
	cfg.fillDefaults()
	return cfg, nil
}

func (c *Config) fillDefaults() {
	if c.Sources == nil {
		c.Sources = make(map[string]Source)
	}
	defaults := map[string]string{
		"dnjournal":       "http://www.dnjournal.com/rss.xml",
		"dnwire":          "https://domainnamewire.com/feed/",
		"namepros":        "https://www.namepros.com/external.php?type=rss2",
		"github_trending": DefaultGitHubTrendingURL,
	}
	for id, url := range defaults {
		s, ok := c.Sources[id]
		if !ok {
			src := Source{Enabled: true, URL: url}
			if id == "github_trending" {
				src.Type = SourceTypeGitHubTrending
			}
			c.Sources[id] = src
			continue
		}
		if id == "github_trending" && strings.TrimSpace(s.Type) == "" {
			s.Type = SourceTypeGitHubTrending
		}
		if s.URL == "" {
			s.URL = url
		}
		c.Sources[id] = s
	}

	a := &c.Analyze
	if a.BaselineDays <= 0 {
		a.BaselineDays = 7
	}
	if a.NewMin <= 0 {
		a.NewMin = 2
	}
	if a.SurgeMin <= 0 {
		a.SurgeMin = 3
	}
	if a.SurgeRatio <= 0 {
		a.SurgeRatio = 3
	}
	if a.TopK <= 0 {
		a.TopK = 15
	}
	if a.CommunityLimit <= 0 {
		a.CommunityLimit = 15
	}
	if a.MinWordLen <= 0 {
		a.MinWordLen = 3
	}
	if a.MaxWordLen <= 0 {
		a.MaxWordLen = 20
	}

	cand := &c.Candidates
	// YAML 未写 enabled 时默认为 true（零值 false 无法区分「未写」与「显式关」）
	// 约定：配置文件里显式写 enabled；fill 只补列表与数值默认。
	if len(cand.TLDs) == 0 {
		cand.TLDs = []string{"com"}
	}
	if cand.MaxPerWord <= 0 {
		cand.MaxPerWord = 10
	}
	if cand.MinHotCount <= 0 {
		cand.MinHotCount = 1
	}
	if len(cand.Prefixes) == 0 {
		cand.Prefixes = []string{"get", "try", "the", "my", "ai"}
	}
	if len(cand.Suffixes) == 0 {
		cand.Suffixes = []string{"hq", "hub", "lab", "app", "ly", "ai"}
	}
	if cand.CheckConcurrency <= 0 {
		cand.CheckConcurrency = 3
	}
	if cand.ReportFreeLimit <= 0 {
		cand.ReportFreeLimit = 30
	}
	if cand.ReportTakenSample <= 0 {
		cand.ReportTakenSample = 5
	}
	if cand.MaxTotal <= 0 {
		cand.MaxTotal = 60
	}
}

// StopwordSet 合并配置停用词与可选文件，返回小写集合。
func (c *Config) StopwordSet(extraFile string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range c.Analyze.Stopwords {
		w = strings.ToLower(strings.TrimSpace(w))
		if w != "" {
			set[w] = true
		}
	}
	if extraFile == "" {
		return set
	}
	b, err := os.ReadFile(extraFile)
	if err != nil {
		return set
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[strings.ToLower(line)] = true
	}
	return set
}

// sourceReady 已启用且可抓取（RSS 需 URL；github_trending 允许空 URL，由默认值补齐）。
func sourceReady(s Source) bool {
	if !s.Enabled {
		return false
	}
	if s.IsGitHubTrending() {
		return true
	}
	return s.URL != ""
}

// EnabledSources 返回已启用源（按默认顺序，其余按名字排序附加）。
func (c *Config) EnabledSources() []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range DefaultSourceOrder {
		if s, ok := c.Sources[id]; ok && sourceReady(s) {
			out = append(out, id)
			seen[id] = true
		}
	}
	var extra []string
	for id, s := range c.Sources {
		if seen[id] || !sourceReady(s) {
			continue
		}
		extra = append(extra, id)
	}
	sort.Strings(extra)
	return append(out, extra...)
}

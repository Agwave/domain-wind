// Package analyze 从 RSS 快照中抽取热词并判定新冒头 / 暴增信号。
package analyze

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"domain-wind/internal/config"
	"domain-wind/internal/fetch"
	"domain-wind/internal/store"
)

// Kind 热词信号类型。
type Kind string

const (
	// KindHot 今日高频（不做历史对比，每天都出）。
	KindHot Kind = "hot"
	// KindNew 新冒头。
	KindNew Kind = "new"
	// KindSurge 暴增。
	KindSurge Kind = "surge"
)

// Evidence 热词证据条目。
type Evidence struct {
	Source string
	Title  string
	Link   string
}

// Signal 一条热词信号。
type Signal struct {
	Word       string
	Kind       Kind
	TodayCount int
	Baseline   float64 // 过去 N 天日均
	Evidence   []Evidence
}

// Result 分析结果。
type Result struct {
	Baseline   bool // 首次运行（无历史快照）
	HotWords   []Signal
	Signals    []Signal
	News       []LabeledItem
	Community  []LabeledItem
	WordCounts map[string]int
}

// LabeledItem 带来源标签的条目。
type LabeledItem struct {
	Source string
	Item   fetch.Item
}

var (
	wordRe = regexp.MustCompile(`[a-z0-9]+`)
	// 标题里的 .factory / .ai / .hiphop 等，常是题材信号
	dotLabelRe = regexp.MustCompile(`\.([a-z][a-z0-9]{1,20})`)
)

// Params 分析参数（从 config 抽取，便于测试）。
type Params struct {
	BaselineDays int
	NewMin       int
	SurgeMin     int
	SurgeRatio   float64
	TopK         int
	MinWordLen   int
	MaxWordLen   int
	Stopwords    map[string]bool
}

// FromConfig 从配置构造 Params（自动合并内置停用词）。
func FromConfig(cfg *config.Config, stopwords map[string]bool) Params {
	a := cfg.Analyze
	return Params{
		BaselineDays: a.BaselineDays,
		NewMin:       a.NewMin,
		SurgeMin:     a.SurgeMin,
		SurgeRatio:   a.SurgeRatio,
		TopK:         a.TopK,
		MinWordLen:   a.MinWordLen,
		MaxWordLen:   a.MaxWordLen,
		Stopwords:    MergeStopwords(stopwords),
	}
}

// Analyze 对比今日快照与历史快照，产出今日高频、热词信号与资讯列表。
func Analyze(today *store.Snapshot, history []*store.Snapshot, p Params) *Result {
	if p.Stopwords == nil {
		p.Stopwords = MergeStopwords(nil)
	}
	res := &Result{
		Baseline:   len(history) == 0,
		WordCounts: map[string]int{},
	}
	if today == nil {
		return res
	}

	res.News, res.Community = splitItems(today)

	todayCounts, todayEvidence := countWords(today, p)
	res.WordCounts = todayCounts
	res.HotWords = rankHotWords(todayCounts, todayEvidence, p)

	if res.Baseline {
		return res
	}

	baselineDays := p.BaselineDays
	if baselineDays <= 0 {
		baselineDays = 7
	}
	if len(history) > baselineDays {
		history = history[:baselineDays]
	}
	histSum := map[string]int{}
	for _, snap := range history {
		counts, _ := countWords(snap, p)
		for w, c := range counts {
			histSum[w] += c
		}
	}
	nDays := float64(len(history))
	if nDays == 0 {
		res.Baseline = true
		return res
	}

	var signals []Signal
	for word, todayN := range todayCounts {
		avg := float64(histSum[word]) / nDays
		kind := Kind("")
		switch {
		case todayN >= p.NewMin && avg < 0.5:
			kind = KindNew
		case todayN >= p.SurgeMin && avg > 0 && float64(todayN) >= avg*p.SurgeRatio:
			kind = KindSurge
		case todayN >= p.SurgeMin && avg == 0 && todayN >= p.NewMin:
			kind = KindNew
		}
		if kind == "" {
			continue
		}
		ev := todayEvidence[word]
		if len(ev) > 3 {
			ev = ev[:3]
		}
		signals = append(signals, Signal{
			Word:       word,
			Kind:       kind,
			TodayCount: todayN,
			Baseline:   avg,
			Evidence:   ev,
		})
	}

	sort.Slice(signals, func(i, j int) bool {
		si, sj := hotScore(signals[i].Word, signals[i].TodayCount), hotScore(signals[j].Word, signals[j].TodayCount)
		if si != sj {
			return si > sj
		}
		return signals[i].Word < signals[j].Word
	})
	if p.TopK > 0 && len(signals) > p.TopK {
		signals = signals[:p.TopK]
	}
	res.Signals = signals
	return res
}

// hotScore 排序分：次数为主，品牌/SLD 形态加权。
func hotScore(word string, count int) int {
	score := count * 100
	if isBrandLike(word) {
		score += 40
	}
	// 极短品牌（jev/kyc/ai）额外加权
	if len(word) >= 3 && len(word) <= 5 && isBrandLike(word) {
		score += 20
	}
	return score
}

// rankHotWords 优先「多次出现的品牌/SLD 形态词」，再补单次专名。
func rankHotWords(counts map[string]int, evidence map[string][]Evidence, p Params) []Signal {
	type pair struct {
		w     string
		n     int
		score int
	}
	var multi, single []pair
	for w, n := range counts {
		if n <= 0 || !keepKeyword(w) {
			continue
		}
		item := pair{w: w, n: n, score: hotScore(w, n)}
		if n >= 2 {
			multi = append(multi, item)
		} else if isBrandLike(w) {
			single = append(single, item)
		}
	}
	sortPairs := func(list []pair) {
		sort.Slice(list, func(i, j int) bool {
			if list[i].score != list[j].score {
				return list[i].score > list[j].score
			}
			return list[i].w < list[j].w
		})
	}
	sortPairs(multi)
	sortPairs(single)

	topK := p.TopK
	if topK <= 0 {
		topK = 15
	}
	list := append([]pair{}, multi...)
	list = append(list, single...)
	if len(list) > topK {
		list = list[:topK]
	}

	out := make([]Signal, 0, len(list))
	for _, it := range list {
		ev := evidence[it.w]
		if len(ev) > 3 {
			ev = ev[:3]
		}
		out = append(out, Signal{
			Word:       it.w,
			Kind:       KindHot,
			TodayCount: it.n,
			Evidence:   ev,
		})
	}
	return out
}

// keepKeyword 丢掉序数词等明显无投资语义的 token。
func keepKeyword(w string) bool {
	if len(w) >= 3 {
		switch {
		case strings.HasSuffix(w, "st"), strings.HasSuffix(w, "nd"), strings.HasSuffix(w, "rd"), strings.HasSuffix(w, "th"):
			prefix := w[:len(w)-2]
			if isAllDigits(prefix) {
				return false
			}
		}
	}
	return true
}

func splitItems(snap *store.Snapshot) (news, community []LabeledItem) {
	for _, src := range config.DefaultSourceOrder {
		items := snap.Sources[src]
		for _, it := range items {
			li := LabeledItem{Source: src, Item: it}
			switch {
			case config.NewsSources[src]:
				news = append(news, li)
			case config.CommunitySources[src]:
				community = append(community, li)
			}
		}
	}
	for src, items := range snap.Sources {
		if config.NewsSources[src] || config.CommunitySources[src] {
			continue
		}
		for _, it := range items {
			news = append(news, LabeledItem{Source: src, Item: it})
		}
	}
	return news, community
}

func countWords(snap *store.Snapshot, p Params) (counts map[string]int, evidence map[string][]Evidence) {
	counts = map[string]int{}
	evidence = map[string][]Evidence{}
	seenDoc := map[string]map[string]bool{}

	for src, items := range snap.Sources {
		for i := range items {
			it := &items[i]
			// 只从标题抽词，避免摘要里版权页脚污染
			words := ExtractWords(it.Title, p)
			docKey := fetch.DedupKey(src, it)
			for _, w := range words {
				if seenDoc[w] == nil {
					seenDoc[w] = map[string]bool{}
				}
				if seenDoc[w][docKey] {
					continue
				}
				seenDoc[w][docKey] = true
				counts[w]++
				if len(evidence[w]) < 3 {
					evidence[w] = append(evidence[w], Evidence{
						Source: src,
						Title:  it.Title,
						Link:   it.Link,
					})
				}
			}
		}
	}
	return counts, evidence
}

// ExtractWords 从文本抽取候选热词（标题友好：含 .label 与普通词）。
func ExtractWords(text string, p Params) []string {
	if p.Stopwords == nil {
		p.Stopwords = MergeStopwords(nil)
	}
	lower := strings.ToLower(text)
	minLen, maxLen := p.MinWordLen, p.MaxWordLen
	if minLen <= 0 {
		minLen = 3
	}
	if maxLen <= 0 {
		maxLen = 20
	}

	seen := map[string]bool{}
	var out []string
	add := func(w string) {
		allowShort := w == "ai" || w == "io" || w == "am"
		if allowShort {
			// 2 字母特例放行
		} else if len(w) < minLen || len(w) > maxLen {
			return
		}
		if !allowShort && len(w) < minLen {
			return
		}
		if len(w) > maxLen {
			return
		}
		if isAllDigits(w) {
			return
		}
		if p.Stopwords[w] {
			return
		}
		if seen[w] {
			return
		}
		seen[w] = true
		out = append(out, w)
	}

	for _, m := range dotLabelRe.FindAllStringSubmatch(lower, -1) {
		add(m[1])
	}
	for _, w := range wordRe.FindAllString(lower, -1) {
		add(w)
	}
	return out
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

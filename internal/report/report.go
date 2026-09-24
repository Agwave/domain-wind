// Package report 生成每日 Markdown 报告。
package report

import (
	"fmt"
	"strings"

	"domain-wind/internal/analyze"
	"domain-wind/internal/avail"
	"domain-wind/internal/config"
)

// SourceLabels 源 ID → 展示名。
var SourceLabels = map[string]string{
	"dnjournal":       "DNJournal",
	"dnwire":          "Domain Name Wire",
	"namepros":        "NamePros",
	"github_trending": "GitHub Trending",
}

// DomainCheck 候选域名粗检结果（写入报告第三节）。
type DomainCheck struct {
	Domain  string
	Word    string
	Pattern string
	Status  avail.Status
	Detail  string
}

// Build 生成完整日报。
func Build(date string, res *analyze.Result, failed []string, communityLimit int, checks []DomainCheck, freeLimit, takenSample int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 域名投资资讯 · %s\n", date)
	b.WriteString("> 数据源：DNJournal / Domain Name Wire / NamePros（RSS）· GitHub Trending · 热词仅从标题抽取，优先品牌/SLD 形态\n\n")
	if res != nil && res.Baseline {
		b.WriteString("> ℹ️ 首次运行：已建立基线；下方「今日高频」可直接看；明日起额外对比新冒头/暴增。\n\n")
	}
	if len(failed) > 0 {
		b.WriteString("> ⚠️ 抓取状态：")
		for i, f := range failed {
			if i > 0 {
				b.WriteString("；")
			}
			b.WriteString(label(f))
			b.WriteString(" 抓取失败（已重试）")
		}
		b.WriteString("\n\n")
	}

	b.WriteString(hotSection(res))
	b.WriteString("\n")
	b.WriteString(communitySection(res, communityLimit))
	if checks != nil {
		b.WriteString("\n")
		b.WriteString(candidatesSection(checks, freeLimit, takenSample))
	}
	return b.String()
}

func candidatesSection(checks []DomainCheck, freeLimit, takenSample int) string {
	var b strings.Builder
	b.WriteString("## 三、候选域名粗检\n\n")
	b.WriteString("> 由今日热词按模板生成（前后缀含 ai）· RDAP+DNS · 非注册商下单保证\n\n")
	if len(checks) == 0 {
		b.WriteString("（无候选）\n")
		return b.String()
	}

	var free, taken, unknown []DomainCheck
	for _, c := range checks {
		switch c.Status {
		case avail.StatusLikelyFree:
			free = append(free, c)
		case avail.StatusTaken:
			taken = append(taken, c)
		default:
			unknown = append(unknown, c)
		}
	}

	b.WriteString("### 可能可注册\n\n")
	if len(free) == 0 {
		b.WriteString("（无）\n\n")
	} else {
		n := len(free)
		if freeLimit > 0 && n > freeLimit {
			n = freeLimit
		}
		for _, c := range free[:n] {
			fmt.Fprintf(&b, "- `%s` ← %s（%s）\n", c.Domain, c.Word, c.Pattern)
		}
		if freeLimit > 0 && len(free) > freeLimit {
			fmt.Fprintf(&b, "\n… 另有 %d 条未列出\n", len(free)-freeLimit)
		}
		b.WriteString("\n")
	}

	b.WriteString("### 已注册（抽样）\n\n")
	if len(taken) == 0 {
		b.WriteString("（无）\n\n")
	} else {
		n := len(taken)
		if takenSample > 0 && n > takenSample {
			n = takenSample
		}
		for _, c := range taken[:n] {
			fmt.Fprintf(&b, "- `%s` ← %s（%s）\n", c.Domain, c.Word, c.Pattern)
		}
		if takenSample > 0 && len(taken) > takenSample {
			fmt.Fprintf(&b, "\n… 共 %d 条已注册\n", len(taken))
		}
		b.WriteString("\n")
	}

	if len(unknown) > 0 {
		fmt.Fprintf(&b, "### 未知（%d）\n\n", len(unknown))
		max := 5
		if len(unknown) < max {
			max = len(unknown)
		}
		for _, c := range unknown[:max] {
			fmt.Fprintf(&b, "- `%s` ← %s · %s\n", c.Domain, c.Word, c.Detail)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func hotSection(res *analyze.Result) string {
	var b strings.Builder
	b.WriteString("## 一、热词信号\n\n")
	if res == nil {
		b.WriteString("（无）\n")
		return b.String()
	}

	if len(res.HotWords) > 0 {
		b.WriteString("### 今日高频\n\n")
		for _, s := range res.HotWords {
			fmt.Fprintf(&b, "- **%s**（%d 条）\n", s.Word, s.TodayCount)
			writeEvidence(&b, s.Evidence)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("今日无可用热词。\n\n")
	}

	if res.Baseline {
		b.WriteString("> 新冒头 / 暴增需历史对比，明日起出现。\n")
		return b.String()
	}

	var news, surges []analyze.Signal
	for _, s := range res.Signals {
		switch s.Kind {
		case analyze.KindNew:
			news = append(news, s)
		case analyze.KindSurge:
			surges = append(surges, s)
		}
	}
	if len(news) == 0 && len(surges) == 0 {
		b.WriteString("今日无新冒头 / 暴增信号。\n")
		return b.String()
	}
	if len(news) > 0 {
		b.WriteString("### 新冒头\n\n")
		for _, s := range news {
			fmt.Fprintf(&b, "- **%s**（今日 %d 条，基线日均 %.1f）\n", s.Word, s.TodayCount, s.Baseline)
			writeEvidence(&b, s.Evidence)
		}
		b.WriteString("\n")
	}
	if len(surges) > 0 {
		b.WriteString("### 暴增\n\n")
		for _, s := range surges {
			fmt.Fprintf(&b, "- **%s**（今日 %d 条，基线日均 %.1f）\n", s.Word, s.TodayCount, s.Baseline)
			writeEvidence(&b, s.Evidence)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func writeEvidence(b *strings.Builder, ev []analyze.Evidence) {
	for _, e := range ev {
		title := e.Title
		if title == "" {
			title = "(无标题)"
		}
		if e.Link != "" {
			fmt.Fprintf(b, "  - [%s](%s) · %s\n", title, e.Link, label(e.Source))
		} else {
			fmt.Fprintf(b, "  - %s · %s\n", title, label(e.Source))
		}
	}
}

func communitySection(res *analyze.Result, limit int) string {
	var b strings.Builder
	b.WriteString("## 二、社区讨论\n\n")
	if res == nil || len(res.Community) == 0 {
		b.WriteString("（无）\n")
		return b.String()
	}
	items := res.Community
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	for i := range items {
		writeItem(&b, &items[i])
	}
	if limit > 0 && len(res.Community) > limit {
		fmt.Fprintf(&b, "\n… 另有 %d 条未列出\n", len(res.Community)-limit)
	}
	return b.String()
}

func writeItem(b *strings.Builder, li *analyze.LabeledItem) {
	title := li.Item.Title
	if title == "" {
		title = "(无标题)"
	}
	if li.Item.Link != "" {
		fmt.Fprintf(b, "- [%s](%s) · %s\n", title, li.Item.Link, label(li.Source))
	} else {
		fmt.Fprintf(b, "- %s · %s\n", title, label(li.Source))
	}
}

func label(id string) string {
	if s, ok := SourceLabels[id]; ok {
		return s
	}
	// 兼容 config 中未登记的源
	_ = config.DefaultSourceOrder
	return id
}

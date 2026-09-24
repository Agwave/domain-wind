package report

import (
	"strings"
	"testing"

	"domain-wind/internal/analyze"
	"domain-wind/internal/avail"
	"domain-wind/internal/fetch"
)

func TestBuildBaseline(t *testing.T) {
	res := &analyze.Result{
		Baseline: true,
		HotWords: []analyze.Signal{
			{Word: "agent", Kind: analyze.KindHot, TodayCount: 2},
		},
		News: []analyze.LabeledItem{
			{Source: "dnwire", Item: fetch.Item{Title: "Hello", Link: "https://x"}},
		},
	}
	md := Build("2026-09-24", res, nil, 20, 15, nil, 0, 0)
	if !strings.Contains(md, "域名投资资讯 · 2026-09-24") {
		t.Error("missing title")
	}
	if !strings.Contains(md, "今日高频") {
		t.Error("missing hot words section")
	}
	if !strings.Contains(md, "agent") {
		t.Error("missing agent")
	}
	if !strings.Contains(md, "Hello") {
		t.Error("missing news")
	}
}

func TestBuildSignals(t *testing.T) {
	res := &analyze.Result{
		Signals: []analyze.Signal{
			{
				Word:       "agent",
				Kind:       analyze.KindNew,
				TodayCount: 3,
				Baseline:   0.1,
				Evidence: []analyze.Evidence{
					{Source: "dnwire", Title: "Agent hot", Link: "https://a"},
				},
			},
		},
	}
	md := Build("2026-09-24", res, []string{"namepros"}, 20, 15, nil, 0, 0)
	if !strings.Contains(md, "新冒头") {
		t.Error("missing new section")
	}
	if !strings.Contains(md, "agent") {
		t.Error("missing word")
	}
	if !strings.Contains(md, "NamePros") && !strings.Contains(md, "抓取失败") {
		t.Error("missing failed note")
	}
}

func TestBuildCandidates(t *testing.T) {
	res := &analyze.Result{HotWords: []analyze.Signal{{Word: "igaming", TodayCount: 2}}}
	checks := []DomainCheck{
		{Domain: "igaminglab.com", Word: "igaming", Pattern: "suffix:lab", Status: avail.StatusLikelyFree},
		{Domain: "igaming.com", Word: "igaming", Pattern: "bare", Status: avail.StatusTaken},
	}
	md := Build("2026-09-24", res, nil, 20, 15, checks, 30, 5)
	if !strings.Contains(md, "候选域名粗检") {
		t.Fatal("missing section")
	}
	if !strings.Contains(md, "igaminglab.com") {
		t.Fatal("missing free candidate")
	}
	if !strings.Contains(md, "可能可注册") {
		t.Fatal("missing free header")
	}
}

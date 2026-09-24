package analyze

import (
	"testing"
	"time"

	"domain-wind/internal/fetch"
	"domain-wind/internal/store"
)

func testParams() Params {
	return Params{
		BaselineDays: 7,
		NewMin:       2,
		SurgeMin:     3,
		SurgeRatio:   3,
		TopK:         15,
		MinWordLen:   3,
		MaxWordLen:   20,
		Stopwords:    MergeStopwords(nil),
	}
}

func snap(date string, items map[string][]fetch.Item) *store.Snapshot {
	return &store.Snapshot{Date: date, FetchedAt: time.Now(), Sources: items}
}

func TestExtractWords(t *testing.T) {
	p := testParams()
	words := ExtractWords("Agent domains and Agent sales — jev brandables!", p)
	set := map[string]bool{}
	for _, w := range words {
		set[w] = true
	}
	if !set["agent"] {
		t.Error("want agent")
	}
	if !set["jev"] {
		t.Error("want jev")
	}
	if !set["brandables"] {
		t.Error("want brandables")
	}
	if set["domain"] || set["domains"] || set["and"] {
		t.Error("stopwords should be filtered")
	}
	if len(ExtractWords("agent agent AGENT", p)) != 1 {
		t.Error("same word once per text")
	}
}

func TestExtractDotLabelAndTitleOnly(t *testing.T) {
	p := testParams()
	words := ExtractWords("Radix applies for .Factory and .AI names", p)
	set := map[string]bool{}
	for _, w := range words {
		set[w] = true
	}
	if !set["factory"] {
		t.Errorf("want factory from .Factory, got %v", words)
	}
	if !set["ai"] {
		t.Errorf("want ai from .AI, got %v", words)
	}

	today := snap("2026-09-24", map[string][]fetch.Item{
		"dnwire": {{
			Title:   "iGaming and KYC brandables surge",
			Link:    "https://t/1",
			Summary: "copyrighted content industry summer opportunity list mark",
		}},
	})
	res := Analyze(today, nil, p)
	got := map[string]bool{}
	for _, s := range res.HotWords {
		got[s.Word] = true
	}
	if !got["igaming"] || !got["kyc"] {
		t.Fatalf("want igaming/kyc from title, got %#v", res.HotWords)
	}
	if got["industry"] || got["summer"] || got["content"] {
		t.Fatalf("summary noise should not count, got %#v", res.HotWords)
	}
}

func TestAnalyzeFirstRun(t *testing.T) {
	today := snap("2026-09-24", map[string][]fetch.Item{
		"dnwire": {
			{Title: "Agent is hot", Link: "https://a/1"},
			{Title: "Agent again", Link: "https://a/2"},
		},
	})
	res := Analyze(today, nil, testParams())
	if !res.Baseline {
		t.Fatal("first run should be baseline")
	}
	if len(res.Signals) != 0 {
		t.Fatalf("baseline should not emit new/surge, got %d", len(res.Signals))
	}
	if len(res.HotWords) == 0 {
		t.Fatal("baseline should still emit today's hot words")
	}
	found := false
	for _, s := range res.HotWords {
		if s.Word == "agent" && s.TodayCount == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("want agent in hot words, got %#v", res.HotWords)
	}
	if len(res.News) != 2 {
		t.Errorf("news items: %d", len(res.News))
	}
}

func TestAnalyzeNewEmerging(t *testing.T) {
	p := testParams()
	// 历史完全没有 agent
	hist := []*store.Snapshot{
		snap("2026-09-23", map[string][]fetch.Item{
			"dnwire": {{Title: "Weekly sales report", Link: "https://h/1"}},
		}),
		snap("2026-09-22", map[string][]fetch.Item{
			"dnjournal": {{Title: "Market update", Link: "https://h/2"}},
		}),
	}
	today := snap("2026-09-24", map[string][]fetch.Item{
		"dnwire": {
			{Title: "Agent domains exploding", Link: "https://t/1", Summary: "agent trend"},
			{Title: "Why Agent brandables sell", Link: "https://t/2"},
		},
		"namepros": {
			{Title: "Buying agent.com?", Link: "https://t/3"},
		},
	})
	res := Analyze(today, hist, p)
	if res.Baseline {
		t.Fatal("should not be baseline")
	}
	found := false
	for _, s := range res.Signals {
		if s.Word == "agent" && s.Kind == KindNew {
			found = true
			if s.TodayCount < 2 {
				t.Errorf("agent count %d", s.TodayCount)
			}
			if len(s.Evidence) == 0 {
				t.Error("want evidence")
			}
		}
	}
	if !found {
		t.Fatalf("want agent as new emerging, signals=%v", res.Signals)
	}
	if len(res.Community) != 1 {
		t.Errorf("community: %d", len(res.Community))
	}
}

func TestAnalyzeSurge(t *testing.T) {
	p := testParams()
	// 历史每天约 1 次 crypto
	hist := []*store.Snapshot{
		snap("2026-09-23", map[string][]fetch.Item{
			"dnwire": {{Title: "crypto sale", Link: "https://h/1"}},
		}),
		snap("2026-09-22", map[string][]fetch.Item{
			"dnwire": {{Title: "crypto note", Link: "https://h/2"}},
		}),
		snap("2026-09-21", map[string][]fetch.Item{
			"dnwire": {{Title: "other news", Link: "https://h/3"}},
		}),
	}
	// 今日 3 条都提 crypto → 日均约 0.67，3 >= 0.67*3
	today := snap("2026-09-24", map[string][]fetch.Item{
		"dnwire": {
			{Title: "crypto one", Link: "https://t/1"},
			{Title: "crypto two", Link: "https://t/2"},
			{Title: "crypto three", Link: "https://t/3"},
		},
	})
	res := Analyze(today, hist, p)
	found := false
	for _, s := range res.Signals {
		if s.Word == "crypto" && s.Kind == KindSurge {
			found = true
			if s.TodayCount != 3 {
				t.Errorf("count=%d", s.TodayCount)
			}
		}
	}
	if !found {
		t.Fatalf("want crypto surge, signals=%v", res.Signals)
	}
}

func TestSameItemWordOnce(t *testing.T) {
	p := testParams()
	hist := []*store.Snapshot{
		snap("2026-09-23", map[string][]fetch.Item{
			"dnwire": {{Title: "quiet day", Link: "https://h/1"}},
		}),
	}
	today := snap("2026-09-24", map[string][]fetch.Item{
		"dnwire": {
			{Title: "agent agent agent", Link: "https://t/1", Summary: "agent agent"},
			{Title: "another agent mention", Link: "https://t/2"},
		},
	})
	res := Analyze(today, hist, p)
	for _, s := range res.Signals {
		if s.Word == "agent" && s.TodayCount != 2 {
			t.Fatalf("same item should count once: got %d", s.TodayCount)
		}
	}
}

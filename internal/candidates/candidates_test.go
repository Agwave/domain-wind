package candidates

import (
	"testing"

	"domain-wind/internal/analyze"
)

func TestGenerateBasic(t *testing.T) {
	p := Params{
		TLDs:       []string{"com"},
		MaxPerWord: 20,
		MinCount:   1,
		Prefixes:   []string{"get", "try", "ai"},
		Suffixes:   []string{"hub", "lab", "ai"},
	}
	hots := []analyze.Signal{
		{Word: "igaming", TodayCount: 2},
		{Word: "ai", TodayCount: 2},
	}
	cands := Generate(hots, &p)
	got := map[string]string{}
	for _, c := range cands {
		got[c.Domain] = c.Word
	}
	want := []string{
		"igaming.com",
		"getigaming.com",
		"tryigaming.com",
		"aiigaming.com",
		"igaminghub.com",
		"igaminglab.com",
		"igamingai.com",
		"ai.com",
	}
	for _, d := range want {
		if _, ok := got[d]; !ok {
			t.Errorf("missing %s in %#v", d, got)
		}
	}
	// 词本身是 ai 时，不再套 ai 前/后缀，避免 aiai.com
	if _, ok := got["aiai.com"]; ok {
		t.Error("should not generate aiai.com")
	}
	if _, ok := got["getai.com"]; !ok {
		t.Error("want getai.com")
	}
}

func TestGenerateMinCountAndDedupe(t *testing.T) {
	p := Params{
		TLDs:       []string{"com"},
		MaxPerWord: 5,
		MinCount:   2,
		Prefixes:   []string{"get"},
		Suffixes:   []string{"hub"},
	}
	hots := []analyze.Signal{
		{Word: "kyc", TodayCount: 1},
		{Word: "igaming", TodayCount: 2},
		{Word: "igaming", TodayCount: 2}, // 不应重复
	}
	cands := Generate(hots, &p)
	for _, c := range cands {
		if c.Word == "kyc" {
			t.Fatalf("kyc count=1 should be filtered: %#v", cands)
		}
	}
	seen := map[string]bool{}
	for _, c := range cands {
		if seen[c.Domain] {
			t.Fatalf("duplicate domain %s", c.Domain)
		}
		seen[c.Domain] = true
	}
}

func TestGenerateMaxPerWord(t *testing.T) {
	p := Params{
		TLDs:       []string{"com"},
		MaxPerWord: 3,
		MinCount:   1,
		Prefixes:   []string{"get", "try", "the", "my", "ai"},
		Suffixes:   []string{"hq", "hub", "lab", "app", "ly", "ai"},
	}
	cands := Generate([]analyze.Signal{{Word: "agentic", TodayCount: 1}}, &p)
	if len(cands) > 3 {
		t.Fatalf("max per word 3, got %d: %#v", len(cands), cands)
	}
	// 光杆应优先保留
	if cands[0].Domain != "agentic.com" {
		t.Fatalf("bare domain should be first, got %s", cands[0].Domain)
	}
}

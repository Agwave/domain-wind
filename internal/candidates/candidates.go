// Package candidates 由热词按模板生成域名候选。
package candidates

import (
	"fmt"
	"strings"
	"unicode"

	"domain-wind/internal/analyze"
)

// Candidate 一条候选域名。
type Candidate struct {
	Domain  string // 如 igaminghub.com
	Word    string // 来源热词
	Pattern string // bare / prefix:get / suffix:hub
}

// Params 生成参数。
type Params struct {
	TLDs       []string
	MaxPerWord int
	MinCount   int
	Prefixes   []string
	Suffixes   []string
}

// DefaultParams 默认模板（前后缀含 ai）。
func DefaultParams() Params {
	return Params{
		TLDs:       []string{"com"},
		MaxPerWord: 10,
		MinCount:   1,
		Prefixes:   []string{"get", "try", "the", "my", "ai"},
		Suffixes:   []string{"hq", "hub", "lab", "app", "ly", "ai"},
	}
}

// Generate 从热词列表生成候选（已按词去重；每词最多 MaxPerWord 条）。
func Generate(hots []analyze.Signal, p *Params) []Candidate {
	if p == nil {
		d := DefaultParams()
		p = &d
	}
	if len(p.TLDs) == 0 {
		p.TLDs = []string{"com"}
	}
	if p.MaxPerWord <= 0 {
		p.MaxPerWord = 10
	}
	if p.MinCount <= 0 {
		p.MinCount = 1
	}

	seenWord := map[string]bool{}
	seenDomain := map[string]bool{}
	var out []Candidate

	for _, h := range hots {
		word := strings.ToLower(strings.TrimSpace(h.Word))
		if word == "" || h.TodayCount < p.MinCount {
			continue
		}
		if seenWord[word] {
			continue
		}
		seenWord[word] = true
		if !usableWord(word) {
			continue
		}

		var batch []Candidate
		for _, tld := range p.TLDs {
			tld = strings.ToLower(strings.Trim(tld, "."))
			if tld == "" {
				continue
			}
			// 1. 光杆
			batch = append(batch, Candidate{
				Domain:  word + "." + tld,
				Word:    word,
				Pattern: "bare",
			})
			// 2. 前缀
			for _, pre := range p.Prefixes {
				pre = strings.ToLower(strings.TrimSpace(pre))
				if pre == "" || skipAffix(word, pre) {
					continue
				}
				batch = append(batch, Candidate{
					Domain:  pre + word + "." + tld,
					Word:    word,
					Pattern: "prefix:" + pre,
				})
			}
			// 3. 后缀
			for _, suf := range p.Suffixes {
				suf = strings.ToLower(strings.TrimSpace(suf))
				if suf == "" || skipAffix(word, suf) {
					continue
				}
				batch = append(batch, Candidate{
					Domain:  word + suf + "." + tld,
					Word:    word,
					Pattern: "suffix:" + suf,
				})
			}
		}

		n := 0
		for _, c := range batch {
			if !validSLD(c.Domain) {
				continue
			}
			if seenDomain[c.Domain] {
				continue
			}
			seenDomain[c.Domain] = true
			out = append(out, c)
			n++
			if n >= p.MaxPerWord {
				break
			}
		}
	}
	return out
}

func skipAffix(word, affix string) bool {
	// 避免 ai + ai → aiai；或词已以该前后缀开头/结尾造成无意义重复
	if word == affix {
		return true
	}
	if strings.HasPrefix(word, affix) && affix == "ai" {
		return true
	}
	if strings.HasSuffix(word, affix) && affix == "ai" {
		return true
	}
	return false
}

func usableWord(w string) bool {
	if len(w) == 2 {
		return w == "ai" || w == "io" || w == "am"
	}
	if len(w) < 3 || len(w) > 20 {
		return false
	}
	for _, r := range w {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func validSLD(domain string) bool {
	parts := strings.Split(domain, ".")
	if len(parts) != 2 {
		return false
	}
	sld, tld := parts[0], parts[1]
	if sld == "" || tld == "" {
		return false
	}
	// 注册商常见 SLD 长度上限约 63
	if len(sld) > 63 {
		return false
	}
	for _, r := range sld {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
			return false
		}
	}
	return true
}

// Domains 抽出域名列表供 check。
func Domains(cands []Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Domain
	}
	return out
}

// IndexByDomain 方便把 check 结果挂回候选。
func IndexByDomain(cands []Candidate) map[string]Candidate {
	m := make(map[string]Candidate, len(cands))
	for _, c := range cands {
		m[c.Domain] = c
	}
	return m
}

// FormatSource 报告里显示来源。
func FormatSource(c Candidate) string {
	return fmt.Sprintf("%s (%s)", c.Word, c.Pattern)
}

// domainwind：域名投资资讯观察工具。
//
// 用法：
//
//	domainwind run                 抓取 RSS → 分析热词 → 生成报告 → 企业微信推送
//	domainwind run --dry-run       同上但不推送
//	domainwind run --date 2026-09-24  用历史日期重跑
//	domainwind fetch               只抓取并入库
//	domainwind report [--date]     用已有快照生成报告（可推送）
//	domainwind list                列出已有快照
//	domainwind check a.com b.com   RDAP+DNS 探测是否已注册
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"domain-wind/internal/analyze"
	"domain-wind/internal/avail"
	"domain-wind/internal/candidates"
	"domain-wind/internal/config"
	"domain-wind/internal/fetch"
	"domain-wind/internal/notify"
	"domain-wind/internal/report"
	"domain-wind/internal/store"
)

const (
	configPath     = "config.yaml"
	localCfgPath   = "config.local.yaml"
	defaultDataDir = "data"
	stopwordsFile  = "data/stopwords.txt"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func usage() {
	fmt.Fprintln(os.Stderr, `domainwind — 域名投资资讯观察工具

用法:
  domainwind run                 抓取 RSS → 分析 → 报告 → 推送
  domainwind fetch               只抓取入库
  domainwind report              用已有快照生成报告（可推送）
  domainwind list                列出已有快照
  domainwind check <domains...>  探测域名是否已注册（RDAP+DNS）

选项:
  --dry-run                    不推送企业微信
  --date YYYY-MM-DD            指定日期（run/report）
  --data-dir DIR               数据目录（默认 data/）
  --file PATH                  check：从文件读域名（每行一个）
  --concurrency N              check：并发数（默认 5）
  --skip-candidates            run/report：跳过热词候选生成与占用粗检`)
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	cmd := args[0]
	opts, positionals := parseArgs(args[1:])
	switch cmd {
	case "run":
		return cmdRun(opts)
	case "fetch":
		return cmdFetch(opts)
	case "report":
		return cmdReport(opts)
	case "list":
		return cmdList(opts)
	case "check":
		return cmdCheck(opts, positionals)
	case "help", "-h", "--help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		usage()
		return 2
	}
}

// parseArgs 解析 --key value / --key=value，并收集位置参数。
func parseArgs(args []string) (opts map[string]string, positionals []string) {
	opts = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			positionals = append(positionals, a)
			continue
		}
		key := strings.TrimPrefix(a, "--")
		if eq := strings.Index(key, "="); eq >= 0 {
			opts[key[:eq]] = key[eq+1:]
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			opts[key] = args[i+1]
			i++
		} else {
			opts[key] = ""
		}
	}
	return opts, positionals
}

func cmdCheck(opts map[string]string, positionals []string) int {
	domains := append([]string{}, positionals...)
	if f := opts["file"]; f != "" {
		lines, err := readDomainFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		domains = append(domains, lines...)
	}
	if len(domains) == 0 {
		fmt.Fprintln(os.Stderr, "请提供域名，例如: domainwind check example.com")
		fmt.Fprintln(os.Stderr, "或: domainwind check --file domains.txt")
		return 2
	}

	c := avail.New()
	if n := opts["concurrency"]; n != "" {
		var conc int
		if _, err := fmt.Sscanf(n, "%d", &conc); err == nil && conc > 0 {
			c.SetConcurrency(conc)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	results := c.CheckAll(ctx, domains)

	var taken, free, unknown int
	for _, r := range results {
		label := avail.StatusLabel(r.Status)
		line := fmt.Sprintf("%-28s %-12s %s", r.Domain, label, r.Detail)
		if r.Err != "" {
			line = fmt.Sprintf("%-28s %-12s %s", r.Domain, label, r.Err)
		}
		fmt.Println(line)
		switch r.Status {
		case avail.StatusTaken:
			taken++
		case avail.StatusLikelyFree:
			free++
		default:
			unknown++
		}
	}
	fmt.Printf("\n合计 %d：已注册 %d · 可能可注册 %d · 未知 %d\n", len(results), taken, free, unknown)
	fmt.Println("说明：RDAP+DNS 粗检，不等于注册商下单保证；保留名/溢价/冻结期需人工确认。")
	return 0
}

func readDomainFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("读取 %s: %w", path, err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}

func cmdRun(opts map[string]string) int {
	cfg, err := config.Load(configPath, localCfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	date := opts["date"]
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	snap, err := fetchAll(ctx, cfg, date)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	dir := dataDir(opts)
	if err := snap.Save(dir); err != nil {
		fmt.Fprintln(os.Stderr, "保存快照失败:", err)
		return 1
	}
	return analyzeReportAndNotify(cfg, snap, date, opts)
}

func cmdFetch(opts map[string]string) int {
	cfg, err := config.Load(configPath, localCfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	date := opts["date"]
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	snap, err := fetchAll(ctx, cfg, date)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := snap.Save(dataDir(opts)); err != nil {
		fmt.Fprintln(os.Stderr, "保存快照失败:", err)
		return 1
	}
	n := 0
	for _, items := range snap.Sources {
		n += len(items)
	}
	fmt.Printf("已保存快照 %s：%d 条，%d 个源失败\n", date, n, len(snap.Failed))
	for _, f := range snap.Failed {
		fmt.Println(" 失败:", f)
	}
	return 0
}

func cmdReport(opts map[string]string) int {
	cfg, err := config.Load(configPath, localCfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	dir := dataDir(opts)
	date := opts["date"]
	if date == "" {
		dates, err := store.ListDates(dir)
		if err != nil || len(dates) == 0 {
			fmt.Fprintln(os.Stderr, "没有可用快照，请先运行 domainwind run / fetch")
			return 1
		}
		date = dates[len(dates)-1]
	}
	snap, err := store.Load(dir, date)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取快照失败:", err)
		return 1
	}
	return analyzeReportAndNotify(cfg, snap, date, opts)
}

func cmdList(opts map[string]string) int {
	dates, err := store.ListDates(dataDir(opts))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, d := range dates {
		fmt.Println(d)
	}
	return 0
}

func dataDir(opts map[string]string) string {
	if d := opts["data-dir"]; d != "" {
		return d
	}
	return defaultDataDir
}

func fetchAll(ctx context.Context, cfg *config.Config, date string) (*store.Snapshot, error) {
	fc := fetch.New()
	snap := &store.Snapshot{
		Date:      date,
		FetchedAt: time.Now(),
		Sources:   map[string][]fetch.Item{},
	}
	ids := cfg.EnabledSources()
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed []string
	)
	for _, id := range ids {
		src := cfg.Sources[id]
		wg.Add(1)
		go func(id, url string) {
			defer wg.Done()
			items, err := fc.FetchSource(ctx, url)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = append(failed, id)
				fmt.Fprintf(os.Stderr, "⚠️ %s: %v\n", id, err)
				return
			}
			snap.Sources[id] = items
		}(id, src.URL)
	}
	wg.Wait()
	sort.Strings(failed)
	snap.Failed = failed
	return snap, nil
}

func analyzeReportAndNotify(cfg *config.Config, snap *store.Snapshot, date string, opts map[string]string) int {
	dir := dataDir(opts)
	history, err := store.LoadRangeBefore(dir, date, cfg.Analyze.BaselineDays)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	stopwords := cfg.StopwordSet(stopwordsFile)
	params := analyze.FromConfig(cfg, stopwords)
	res := analyze.Analyze(snap, history, params)

	var checks []report.DomainCheck
	if _, skip := opts["skip-candidates"]; !skip && cfg.Candidates.Enabled {
		checks = runCandidateChecks(cfg, res)
	}

	md := report.Build(date, res, snap.Failed, cfg.Analyze.NewsLimit, cfg.Analyze.CommunityLimit,
		checks, cfg.Candidates.ReportFreeLimit, cfg.Candidates.ReportTakenSample)

	reportDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportDir, 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(reportDir, date+".md"), []byte(md), 0o644)
	}
	fmt.Println(md)

	hasContent := len(res.HotWords) > 0 || len(res.Signals) > 0 || len(res.News) > 0 || len(res.Community) > 0

	if _, dry := opts["dry-run"]; !dry && cfg.Notify.WebhookURL != "" {
		if !hasContent && !cfg.Notify.NotifyWhenQuiet && !res.Baseline {
			fmt.Println("ℹ️ 今日无热词且无新资讯，不推送（notify_when_quiet=false）")
		} else {
			if err := notifyAll(cfg, date, hasContent, res.Baseline, md, dir); err != nil {
				fmt.Fprintln(os.Stderr, "推送失败:", err)
				return 1
			}
		}
	}
	return 0
}

func runCandidateChecks(cfg *config.Config, res *analyze.Result) []report.DomainCheck {
	if res == nil {
		return nil
	}
	p := candidates.Params{
		TLDs:       cfg.Candidates.TLDs,
		MaxPerWord: cfg.Candidates.MaxPerWord,
		MinCount:   cfg.Candidates.MinHotCount,
		Prefixes:   cfg.Candidates.Prefixes,
		Suffixes:   cfg.Candidates.Suffixes,
	}
	cands := candidates.Generate(res.HotWords, &p)
	if cfg.Candidates.MaxTotal > 0 && len(cands) > cfg.Candidates.MaxTotal {
		cands = cands[:cfg.Candidates.MaxTotal]
	}
	if len(cands) == 0 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "ℹ️ 由热词生成 %d 条候选，开始占用粗检…\n", len(cands))

	client := avail.New()
	client.SetConcurrency(cfg.Candidates.CheckConcurrency)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	results := client.CheckAll(ctx, candidates.Domains(cands))

	byDomain := candidates.IndexByDomain(cands)
	out := make([]report.DomainCheck, 0, len(results))
	for _, r := range results {
		c := byDomain[r.Domain]
		out = append(out, report.DomainCheck{
			Domain:  r.Domain,
			Word:    c.Word,
			Pattern: c.Pattern,
			Status:  r.Status,
			Detail:  r.Detail,
		})
	}
	return out
}

func notifyAll(cfg *config.Config, date string, hasContent, baseline bool, md, dir string) error {
	w := notify.New(cfg.Notify.WebhookURL)
	if w == nil {
		return nil
	}
	footer := fmt.Sprintf("\n---\n完整报告：data/reports/%s.md", date)

	var msgs []string
	kind := "report"
	switch {
	case baseline:
		// 首日也推完整报告（含今日高频）
		kind = "baseline"
		var err error
		msgs, err = notify.BuildMessages("", md, footer)
		if err != nil {
			return err
		}
	case !hasContent:
		kind = "quiet"
		header := fmt.Sprintf("# 域名投资资讯 · %s", date)
		msgs = []string{header + "\n\n今日无热词信号，也无新资讯。" + footer}
	default:
		var err error
		msgs, err = notify.BuildMessages("", md, footer)
		if err != nil {
			return err
		}
	}
	sum := sha256.Sum256([]byte(md + "\n" + kind))
	hash := hex.EncodeToString(sum[:8])
	st := store.LoadState(dir)
	if st.LastSnapshotDate == date && st.LastNotifiedHash == hash {
		fmt.Println("ℹ️ 今天已推送过相同内容，跳过")
		return nil
	}
	if err := w.SendAll(msgs); err != nil {
		return err
	}
	return store.SaveState(dir, store.State{LastSnapshotDate: date, LastNotifiedHash: hash})
}

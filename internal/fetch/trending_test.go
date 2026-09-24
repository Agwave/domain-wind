package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseTrendingHTML(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "github_trending.html"))
	if err != nil {
		t.Fatal(err)
	}
	items, err := ParseTrendingHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}

	want := []struct {
		title, link, summary string
	}{
		{
			title:   "anthropics/financial-services",
			link:    "https://github.com/anthropics/financial-services",
			summary: "",
		},
		{
			title:   "google/ax",
			link:    "https://github.com/google/ax",
			summary: "Google's open agentic orchestration runtime",
		},
		{
			title:   "davila7/claude-code-templates",
			link:    "https://github.com/davila7/claude-code-templates",
			summary: "CLI tool for configuring and monitoring Claude Code",
		},
	}
	for i, w := range want {
		it := items[i]
		if it.Title != w.title {
			t.Errorf("item %d title: got %q want %q", i, it.Title, w.title)
		}
		if it.Link != w.link {
			t.Errorf("item %d link: got %q want %q", i, it.Link, w.link)
		}
		if it.Summary != w.summary {
			t.Errorf("item %d summary: got %q want %q", i, it.Summary, w.summary)
		}
		if it.ID == "" {
			t.Errorf("item %d missing id", i)
		}
		if it.Published.IsZero() {
			t.Errorf("item %d missing published", i)
		}
	}
}

func TestParseTrendingHTMLEmpty(t *testing.T) {
	_, err := ParseTrendingHTML([]byte(`<!DOCTYPE html><html><body><p>no repos</p></body></html>`))
	if err == nil {
		t.Fatal("want error for empty trending page")
	}
}

func TestFetchGitHubTrendingRetries(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "github_trending.html"))
	if err != nil {
		t.Fatal(err)
	}

	oldRetries, oldBase, oldCap := trendingRetries, trendingBackoffBase, trendingBackoffCap
	trendingRetries = 4
	trendingBackoffBase = time.Millisecond
	trendingBackoffCap = time.Millisecond
	defer func() {
		trendingRetries = oldRetries
		trendingBackoffBase = oldBase
		trendingBackoffCap = oldCap
	}()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n <= 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New()
	items, err := c.FetchGitHubTrending(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchGitHubTrending: %v (hits=%d)", err, hits.Load())
	}
	if hits.Load() != 4 {
		t.Fatalf("hits=%d want 4", hits.Load())
	}
	if len(items) != 3 {
		t.Fatalf("items=%d want 3", len(items))
	}
}

func TestFetchGitHubTrendingPermanentHTTP(t *testing.T) {
	oldRetries, oldBase := trendingRetries, trendingBackoffBase
	trendingRetries = 3
	trendingBackoffBase = time.Millisecond
	defer func() {
		trendingRetries = oldRetries
		trendingBackoffBase = oldBase
	}()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New()
	_, err := c.FetchGitHubTrending(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("want error")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d want 1 (no retry on 404)", hits.Load())
	}
}

func TestTrendingBackoff(t *testing.T) {
	oldBase, oldCap := trendingBackoffBase, trendingBackoffCap
	trendingBackoffBase = 2 * time.Second
	trendingBackoffCap = 16 * time.Second
	defer func() {
		trendingBackoffBase = oldBase
		trendingBackoffCap = oldCap
	}()
	if g, w := trendingBackoff(1), 2*time.Second; g != w {
		t.Fatalf("attempt1=%v want %v", g, w)
	}
	if g, w := trendingBackoff(2), 4*time.Second; g != w {
		t.Fatalf("attempt2=%v want %v", g, w)
	}
	if g, w := trendingBackoff(5), 16*time.Second; g != w {
		t.Fatalf("attempt5=%v want %v", g, w)
	}
}

package avail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"Example.COM", "example.com", true},
		{"  foo.ai ", "foo.ai", true},
		{"http://bar.com/path", "bar.com", true},
		{"https://www.baz.io", "baz.io", true},
		{"nodot", "", false},
		{"", "", false},
		{".com", "", false},
	}
	for _, c := range cases {
		got, err := NormalizeDomain(c.in)
		if c.ok {
			if err != nil || got != c.want {
				t.Errorf("NormalizeDomain(%q)=%q,%v want %q", c.in, got, err, c.want)
			}
		} else if err == nil {
			t.Errorf("NormalizeDomain(%q) should fail, got %q", c.in, got)
		}
	}
}

func TestDecide(t *testing.T) {
	cases := []struct {
		name   string
		rdap   rdapOutcome
		hasNS  bool
		dnsErr bool
		want   Status
	}{
		{"rdap taken", rdapTaken, false, false, StatusTaken},
		{"ns taken", rdapNotFound, true, false, StatusTaken},
		{"both free signals", rdapNotFound, false, false, StatusLikelyFree},
		{"rdap error no ns", rdapError, false, false, StatusUnknown},
		{"rdap error has ns", rdapError, true, false, StatusTaken},
		{"rdap free dns error", rdapNotFound, false, true, StatusUnknown},
	}
	for _, c := range cases {
		got := decide(c.rdap, c.hasNS, c.dnsErr)
		if got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestCheckRDAPWithMock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/domain/taken.com", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rdap+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"objectClassName":"domain","ldhName":"taken.com"}`))
	})
	mux.HandleFunc("/domain/free.com", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorCode":404}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := New()
	c.rdapBase = strings.TrimRight(srv.URL, "/") + "/domain"
	c.skipDNS = true
	c.http = srv.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r1 := c.Check(ctx, "taken.com")
	if r1.Status != StatusTaken {
		t.Fatalf("taken.com: %#v", r1)
	}
	r2 := c.Check(ctx, "free.com")
	if r2.Status != StatusLikelyFree {
		t.Fatalf("free.com: %#v", r2)
	}
}

package fetch

import (
	"strings"
	"testing"
)

func TestParseList(t *testing.T) {
	xml := `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Test</title>
    <item>
      <title>Agent domains surge</title>
      <link>https://example.com/1</link>
      <guid>https://example.com/1</guid>
      <description>The word agent is hot.</description>
      <pubDate>Mon, 22 Sep 2026 10:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Jev brandables</title>
      <link>https://example.com/2</link>
      <description><![CDATA[<p>Jev sales rising</p>]]></description>
      <pubDate>Tue, 23 Sep 2026 12:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`
	items, err := ParseFeed([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Title != "Agent domains surge" {
		t.Errorf("title: %q", items[0].Title)
	}
	if items[0].Link != "https://example.com/1" {
		t.Errorf("link: %q", items[0].Link)
	}
	if !strings.Contains(items[1].Summary, "Jev sales rising") {
		t.Errorf("summary should strip HTML: %q", items[1].Summary)
	}
	if items[0].Published.IsZero() {
		t.Error("published should parse")
	}
}

func TestParseSingleEntry(t *testing.T) {
	xml := `<?xml version="1.0"?>
<rss version="2.0"><channel><title>T</title>
<item><title>Only one</title><link>https://ex.com/a</link><description>hi</description></item>
</channel></rss>`
	items, err := ParseFeed([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1, got %d", len(items))
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := ParseFeed([]byte("")); err == nil {
		t.Fatal("empty should error")
	}
	if _, err := ParseFeed([]byte("not xml {{{")); err == nil {
		t.Fatal("invalid should error")
	}
}

func TestParseAtom(t *testing.T) {
	xml := `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>Atom item</title>
    <id>urn:1</id>
    <link href="https://example.com/atom" rel="alternate"/>
    <summary>hello agent world</summary>
    <updated>2026-09-22T10:00:00Z</updated>
  </entry>
</feed>`
	items, err := ParseFeed([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1, got %d", len(items))
	}
	if items[0].Link != "https://example.com/atom" {
		t.Errorf("link: %q", items[0].Link)
	}
}

func TestDedupKey(t *testing.T) {
	k1 := DedupKey("dnwire", &Item{Link: "https://a", Title: "T"})
	k2 := DedupKey("dnwire", &Item{Title: "T"})
	if k1 != "dnwire|https://a" {
		t.Errorf("got %q", k1)
	}
	if k2 != "dnwire|T" {
		t.Errorf("got %q", k2)
	}
}

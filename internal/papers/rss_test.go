package papers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- RSS 2.0 fixtures -------------------------------------------------------

const rss2Feed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Blog</title>
    <item>
      <title>First Post</title>
      <link>https://example.com/first</link>
      <description>&lt;p&gt;This is the &lt;b&gt;abstract&lt;/b&gt; of the first post.&lt;/p&gt;</description>
      <pubDate>Mon, 10 Mar 2025 09:00:00 +0000</pubDate>
      <dc:creator>Alice</dc:creator>
    </item>
    <item>
      <title>Second Post</title>
      <link>https://example.com/second</link>
      <description>Plain text description.</description>
      <pubDate>Tue, 11 Mar 2025 10:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

// --- Atom 1.0 fixtures ------------------------------------------------------

const atomFeed1 = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Simon's Blog</title>
  <entry>
    <title>LLM Thoughts</title>
    <link rel="alternate" href="https://simonwillison.net/2025/Mar/10/llm/"/>
    <summary type="html">&lt;p&gt;Some &lt;em&gt;thoughts&lt;/em&gt; on LLMs.&lt;/p&gt;</summary>
    <published>2025-03-10T09:00:00Z</published>
    <author><name>Simon Willison</name></author>
  </entry>
  <entry>
    <title>No Link Entry</title>
    <summary>This entry has no link and should be skipped.</summary>
    <published>2025-03-11T10:00:00Z</published>
  </entry>
</feed>`

// --- helpers ----------------------------------------------------------------

func newRSSServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func newRSSFetcherForTest(t *testing.T, srv *httptest.Server, sourceName string) *RSSFetcher {
	t.Helper()
	f := &RSSFetcher{
		httpClient: srv.Client(),
		feedURL:    srv.URL,
		sourceName: sourceName,
		maxItems:   10,
	}
	return f
}

// --- tests ------------------------------------------------------------------

func TestRSSFetcher_RSS2_ParsesItems(t *testing.T) {
	srv := newRSSServer(t, rss2Feed, http.StatusOK)
	defer srv.Close()

	f := newRSSFetcherForTest(t, srv, "testblog")
	ps, err := f.Fetch(context.Background(), "BLOGS")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(ps) != 2 {
		t.Fatalf("expected 2 papers, got %d", len(ps))
	}

	p := ps[0]
	if p.Title != "First Post" {
		t.Errorf("Title = %q, want %q", p.Title, "First Post")
	}
	if p.URL != "https://example.com/first" {
		t.Errorf("URL = %q", p.URL)
	}
	if p.Authors[0] != "Alice" {
		t.Errorf("Author = %q, want Alice", p.Authors[0])
	}
	// HTML should be stripped from description.
	if strings.Contains(p.Abstract, "<") {
		t.Errorf("Abstract still contains HTML: %q", p.Abstract)
	}
	if !strings.Contains(p.Abstract, "abstract") {
		t.Errorf("Abstract should contain 'abstract': %q", p.Abstract)
	}
	if p.Source != "testblog" {
		t.Errorf("Source = %q, want testblog", p.Source)
	}
	if p.Date.IsZero() {
		t.Error("Date should not be zero")
	}
}

func TestRSSFetcher_RSS2_FallbackAuthor(t *testing.T) {
	srv := newRSSServer(t, rss2Feed, http.StatusOK)
	defer srv.Close()

	f := newRSSFetcherForTest(t, srv, "testblog")
	ps, err := f.Fetch(context.Background(), "BLOGS")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Second item has no dc:creator — should fall back to sourceName.
	if ps[1].Authors[0] != "testblog" {
		t.Errorf("fallback author = %q, want testblog", ps[1].Authors[0])
	}
}

func TestRSSFetcher_Atom_ParsesEntries(t *testing.T) {
	srv := newRSSServer(t, atomFeed1, http.StatusOK)
	defer srv.Close()

	f := newRSSFetcherForTest(t, srv, "simonwillison")
	ps, err := f.Fetch(context.Background(), "BLOGS")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Entry with no link should be skipped → only 1 paper.
	if len(ps) != 1 {
		t.Fatalf("expected 1 paper (no-link entry skipped), got %d", len(ps))
	}

	p := ps[0]
	if p.Title != "LLM Thoughts" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.Authors[0] != "Simon Willison" {
		t.Errorf("Author = %q", p.Authors[0])
	}
	if strings.Contains(p.Abstract, "<") {
		t.Errorf("Abstract still contains HTML: %q", p.Abstract)
	}
	wantDate := time.Date(2025, 3, 10, 9, 0, 0, 0, time.UTC)
	if !p.Date.Equal(wantDate) {
		t.Errorf("Date = %v, want %v", p.Date, wantDate)
	}
}

func TestRSSFetcher_HTTPError(t *testing.T) {
	srv := newRSSServer(t, "", http.StatusServiceUnavailable)
	defer srv.Close()

	f := newRSSFetcherForTest(t, srv, "testblog")
	_, err := f.Fetch(context.Background(), "BLOGS")
	if err == nil {
		t.Fatal("expected error for 503")
	}
}

func TestRSSFetcher_InvalidXML(t *testing.T) {
	srv := newRSSServer(t, "not xml at all", http.StatusOK)
	defer srv.Close()

	f := newRSSFetcherForTest(t, srv, "testblog")
	_, err := f.Fetch(context.Background(), "BLOGS")
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestRSSFetcher_UnknownFormat(t *testing.T) {
	srv := newRSSServer(t, `<document><item/></document>`, http.StatusOK)
	defer srv.Close()

	f := newRSSFetcherForTest(t, srv, "testblog")
	_, err := f.Fetch(context.Background(), "BLOGS")
	if err == nil {
		t.Fatal("expected error for unknown XML format")
	}
	if !strings.Contains(err.Error(), "unknown feed format") {
		t.Errorf("error should mention unknown format: %v", err)
	}
}

func TestRSSFetcher_MaxItems(t *testing.T) {
	// Build a feed with 15 items.
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel>`)
	for i := range 15 {
		sb.WriteString(`<item><title>Post `)
		sb.WriteString(strings.Repeat("x", i))
		sb.WriteString(`</title><link>https://example.com/`)
		sb.WriteString(strings.Repeat("x", i+1))
		sb.WriteString(`</link></item>`)
	}
	sb.WriteString(`</channel></rss>`)

	srv := newRSSServer(t, sb.String(), http.StatusOK)
	defer srv.Close()

	f := newRSSFetcherForTest(t, srv, "testblog")
	f.maxItems = 5
	ps, err := f.Fetch(context.Background(), "BLOGS")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(ps) != 5 {
		t.Errorf("expected 5 papers (maxItems), got %d", len(ps))
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<p>Hello <b>world</b>!</p>", "Hello world !"},
		{"No tags here", "No tags here"},
		{"&lt;escaped&gt; &amp; entities", "<escaped> & entities"},
		{"  extra   spaces  ", "extra spaces"},
	}
	for _, tt := range tests {
		got := rssAbstract(tt.input)
		if got != tt.want {
			t.Errorf("rssAbstract(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseRSSDate(t *testing.T) {
	tests := []struct {
		input   string
		wantZero bool
	}{
		{"Mon, 10 Mar 2025 09:00:00 +0000", false},
		{"Mon, 10 Mar 2025 09:00:00 GMT", false},
		{"", true},
		{"not a date", true},
	}
	for _, tt := range tests {
		got := parseRSSDate(tt.input)
		if tt.wantZero && !got.IsZero() {
			t.Errorf("parseRSSDate(%q): expected zero time, got %v", tt.input, got)
		}
		if !tt.wantZero && got.IsZero() {
			t.Errorf("parseRSSDate(%q): expected non-zero time, got zero", tt.input)
		}
	}
}

func TestParseAtomDate(t *testing.T) {
	got := parseAtomDate("2025-03-10T09:00:00Z")
	if got.IsZero() {
		t.Error("expected non-zero time for valid RFC3339")
	}
	if got.Year() != 2025 {
		t.Errorf("year = %d, want 2025", got.Year())
	}
}

func TestNewRSSFetcher_PanicsOnHTTP(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for http:// URL")
		}
	}()
	NewRSSFetcher("http://insecure.example.com/feed", "test")
}

package papers

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// RSSFetcher retrieves articles from an RSS 2.0 or Atom 1.0 feed and maps
// them to Paper so they integrate with the existing briefing pipeline.
// It implements SourceFetcher.
type RSSFetcher struct {
	httpClient *http.Client
	feedURL    string
	sourceName string // used as Paper.Source, e.g. "simonwillison"
	maxItems   int
}

// NewRSSFetcher creates an RSSFetcher for the given feed URL.
// Panics if feedURL is not HTTPS (constructor-time enforcement).
func NewRSSFetcher(feedURL, sourceName string) *RSSFetcher {
	if err := validateHTTPS(feedURL); err != nil {
		panic("rss " + sourceName + ": " + err.Error())
	}
	return &RSSFetcher{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		feedURL:    feedURL,
		sourceName: sourceName,
		maxItems:   10,
	}
}

// Fetch retrieves the RSS/Atom feed and returns up to maxItems as Papers.
// The topic argument is ignored — all articles from the feed are returned.
func (f *RSSFetcher) Fetch(ctx context.Context, _ string) ([]Paper, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("rss %s: build request: %w", f.sourceName, err)
	}
	req.Header.Set("User-Agent", "Beacon/1.0 (research aggregator; RSS reader)")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml")

	resp, err := f.httpClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("rss %s: get feed: %w", f.sourceName, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rss %s: status %d", f.sourceName, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB cap
	if err != nil {
		return nil, fmt.Errorf("rss %s: read body: %w", f.sourceName, err)
	}

	ps, err := f.parse(body)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "rss: fetched", "source", f.sourceName, "count", len(ps))
	return ps, nil
}

// parse detects the feed format (RSS 2.0 vs Atom 1.0) and delegates.
func (f *RSSFetcher) parse(data []byte) ([]Paper, error) {
	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("rss %s: detect format: %w", f.sourceName, err)
	}
	switch root.XMLName.Local {
	case "rss":
		return f.parseRSS(data)
	case "feed":
		return f.parseAtom(data)
	default:
		return nil, fmt.Errorf("rss %s: unknown feed format %q", f.sourceName, root.XMLName.Local)
	}
}

// --- RSS 2.0 ---------------------------------------------------------------

type rssDoc struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	// dc:creator — Dublin Core namespace
	Creator string `xml:"http://purl.org/dc/elements/1.1/ creator"`
}

func (f *RSSFetcher) parseRSS(data []byte) ([]Paper, error) {
	var doc rssDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("rss %s: parse RSS 2.0: %w", f.sourceName, err)
	}

	items := doc.Channel.Items
	if len(items) > f.maxItems {
		items = items[:f.maxItems]
	}

	ps := make([]Paper, 0, len(items))
	for _, item := range items {
		author := item.Creator
		if author == "" {
			author = f.sourceName
		}
		ps = append(ps, Paper{
			Title:    strings.TrimSpace(item.Title),
			Authors:  []string{author},
			Source:   f.sourceName,
			Date:     parseRSSDate(item.PubDate),
			URL:      strings.TrimSpace(item.Link),
			Abstract: rssAbstract(item.Description),
		})
	}
	return ps, nil
}

// --- Atom 1.0 ---------------------------------------------------------------

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Author    atomAuthor `xml:"author"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

func (f *RSSFetcher) parseAtom(data []byte) ([]Paper, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("rss %s: parse Atom 1.0: %w", f.sourceName, err)
	}

	entries := feed.Entries
	if len(entries) > f.maxItems {
		entries = entries[:f.maxItems]
	}

	ps := make([]Paper, 0, len(entries))
	for _, e := range entries {
		href := atomAlternateLink(e.Links)
		if href == "" {
			continue
		}
		dateStr := e.Published
		if dateStr == "" {
			dateStr = e.Updated
		}
		author := e.Author.Name
		if author == "" {
			author = f.sourceName
		}
		// Prefer summary over full content for the abstract excerpt.
		body := e.Summary
		if body == "" {
			body = e.Content
		}
		ps = append(ps, Paper{
			Title:    strings.TrimSpace(e.Title),
			Authors:  []string{author},
			Source:   f.sourceName,
			Date:     parseAtomDate(dateStr),
			URL:      href,
			Abstract: rssAbstract(body),
		})
	}
	return ps, nil
}

// atomAlternateLink returns the href of the first alternate or untyped link.
func atomAlternateLink(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "alternate" || l.Rel == "" {
			return l.Href
		}
	}
	return ""
}

// --- helpers ----------------------------------------------------------------

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// rssAbstract strips HTML tags, unescapes entities, normalises whitespace,
// and truncates to 500 characters — suitable as a Paper.Abstract.
func rssAbstract(s string) string {
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return s
}

var rssDateLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	"Mon, 02 Jan 2006 15:04:05 GMT",
	"02 Jan 2006 15:04:05 -0700",
	"02 Jan 2006 15:04:05 GMT",
}

func parseRSSDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range rssDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseAtomDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

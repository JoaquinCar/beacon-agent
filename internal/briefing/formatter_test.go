package briefing

import (
	"strings"
	"testing"
	"time"

	"github.com/joako/beacon/internal/papers"
)

func makeSection(title, tldr, body, url string, authors []string) Section {
	return Section{
		Paper: papers.Paper{
			Title:   title,
			Authors: authors,
			Source:  "arxiv",
			Date:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			URL:     url,
		},
		TLDr: tldr,
		Body: body,
	}
}

func TestFormat_SummaryMode_ContainsLightning(t *testing.T) {
	b := Briefing{
		Mode: ModeSummary,
		Sections: []Section{
			makeSection("Paper A", "", "5 line summary here.", "https://arxiv.org/abs/1", []string{"Alice"}),
		},
	}

	out := Format(b)

	if !strings.Contains(out, "📄 Paper A") {
		t.Error("missing title")
	}
	if !strings.Contains(out, "⚡") {
		t.Error("missing lightning emoji for summary mode")
	}
	if !strings.Contains(out, "5 line summary here.") {
		t.Error("missing summary body")
	}
	if !strings.Contains(out, "🔗 https://arxiv.org/abs/1") {
		t.Error("missing link — link must always be present")
	}
}

func TestFormat_FullMode_ContainsTLDrAndAnalysis(t *testing.T) {
	b := Briefing{
		Mode: ModeFull,
		Sections: []Section{
			makeSection("Paper B", "This paper solves X.", "Motivation: ...\nMethod: ...", "https://arxiv.org/abs/2", []string{"Bob", "Carol"}),
		},
	}

	out := Format(b)

	if !strings.Contains(out, "💡 TL;DR: This paper solves X.") {
		t.Error("missing TL;DR line")
	}
	if !strings.Contains(out, "Motivation: ...") {
		t.Error("missing analysis body")
	}
	if !strings.Contains(out, "🔗 https://arxiv.org/abs/2") {
		t.Error("missing link — link must always be present in full mode too")
	}
}

func TestFormat_AlwaysIncludesLink_BothModes(t *testing.T) {
	url := "https://arxiv.org/abs/999"
	for _, mode := range []Mode{ModeSummary, ModeFull} {
		b := Briefing{
			Mode:     mode,
			Sections: []Section{makeSection("T", "TL", "body", url, []string{"A"})},
		}
		out := Format(b)
		if !strings.Contains(out, "🔗 "+url) {
			t.Errorf("mode %v: link missing from output", mode)
		}
	}
}

func TestFormat_SeparatorBetweenSections(t *testing.T) {
	b := Briefing{
		Mode: ModeSummary,
		Sections: []Section{
			makeSection("P1", "", "body1", "https://u1", []string{"A"}),
			makeSection("P2", "", "body2", "https://u2", []string{"B"}),
		},
	}
	out := Format(b)
	if strings.Count(out, "---") < 2 {
		t.Errorf("expected at least 2 separators for 2 sections, got: %s", out)
	}
}

func TestByline_TruncatesAuthors(t *testing.T) {
	sec := makeSection("T", "", "", "", []string{"A", "B", "C", "D", "E"})
	bl := byline(sec)
	if !strings.Contains(bl, "et al.") {
		t.Errorf("expected 'et al.' for >3 authors, got: %s", bl)
	}
	if strings.Contains(bl, "D") || strings.Contains(bl, "E") {
		t.Errorf("4th and 5th authors should be truncated, got: %s", bl)
	}
}

func TestByline_UnknownDate(t *testing.T) {
	sec := Section{
		Paper: papers.Paper{Title: "T", Authors: []string{"A"}, Source: "arxiv"},
	}
	bl := byline(sec)
	if !strings.Contains(bl, "unknown date") {
		t.Errorf("expected 'unknown date' for zero time, got: %s", bl)
	}
}

func TestFormat_ModeString(t *testing.T) {
	if ModeFull.String() != "full" {
		t.Errorf("ModeFull.String() = %q, want \"full\"", ModeFull.String())
	}
	if ModeSummary.String() != "summary" {
		t.Errorf("ModeSummary.String() = %q, want \"summary\"", ModeSummary.String())
	}
}

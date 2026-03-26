package briefing

import (
	"fmt"
	"strings"
)

// Format renders a Briefing as a human-readable string using the emoji format
// defined in CLAUDE.md. Both modes always include the paper link.
func Format(b Briefing) string {
	var sb strings.Builder
	for _, sec := range b.Sections {
		sb.WriteString(formatSection(sec, b.Mode))
		sb.WriteString("\n---\n\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatSection(sec Section, mode Mode) string {
	var sb strings.Builder

	// Header: title + byline
	fmt.Fprintf(&sb, "📄 %s\n", sec.Paper.Title)
	fmt.Fprintf(&sb, "🏷️  %s\n", byline(sec))

	if mode == ModeSummary {
		// HIGH_BPM: punchy 5–8 line summary + link
		fmt.Fprintf(&sb, "\n⚡ %s\n", sec.Body)
	} else {
		// NORMAL: TL;DR + full structured analysis + link
		if sec.TLDr != "" {
			fmt.Fprintf(&sb, "\n💡 TL;DR: %s\n", sec.TLDr)
		}
		if sec.Body != "" {
			sb.WriteString("\n")
			sb.WriteString(sec.Body)
			sb.WriteString("\n")
		}
	}

	// Always include the link — both modes.
	if sec.Paper.URL != "" {
		fmt.Fprintf(&sb, "\n🔗 %s\n", sec.Paper.URL)
	}

	return sb.String()
}

// byline formats "Author1, Author2 et al. · source · date".
func byline(sec Section) string {
	authors := sec.Paper.Authors
	authorStr := "(unknown)"
	if len(authors) > 0 {
		if len(authors) > 3 {
			authorStr = strings.Join(authors[:3], ", ") + " et al."
		} else {
			authorStr = strings.Join(authors, ", ")
		}
	}

	date := "(unknown date)"
	if !sec.Paper.Date.IsZero() {
		date = sec.Paper.Date.Format("2006-01-02")
	}

	return fmt.Sprintf("%s · %s · %s", authorStr, sec.Paper.Source, date)
}

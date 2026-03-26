package briefing

import (
	"time"

	"github.com/joako/beacon/internal/papers"
)

// Mode controls how briefings are generated and formatted.
type Mode int

const (
	ModeFull    Mode = iota // NORMAL mood: full analysis + TL;DR
	ModeSummary             // HIGH_BPM mood: 5–8 line summary
)

func (m Mode) String() string {
	if m == ModeSummary {
		return "summary"
	}
	return "full"
}

// Section holds one paper together with its Claude-generated content.
type Section struct {
	Paper papers.Paper
	TLDr  string // ModeFull only: 1-sentence overview
	Body  string // ModeFull: structured analysis; ModeSummary: 5–8 line summary
}

// Briefing is the complete output of a generation run, ready for formatting.
type Briefing struct {
	Mode        Mode
	Sections    []Section
	GeneratedAt time.Time
}

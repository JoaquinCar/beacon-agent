package briefing

import (
	"time"

	"github.com/joako/beacon/internal/papers"
)

// Briefing is the generated output ready for delivery.
type Briefing struct {
	ID          string
	Papers      []papers.Paper
	GeneratedAt time.Time
	Mode        string // "summary" | "full"
	Body        string // rendered markdown/HTML
}

package papers

import "time"

// Paper represents a single academic paper fetched from any source.
type Paper struct {
	Title    string
	Authors  []string
	Source   string // "arxiv" | "huggingface" | "semantic_scholar" | "biorxiv"
	Date     time.Time
	URL      string
	Abstract string
}

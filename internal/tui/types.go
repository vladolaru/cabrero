package tui

// CitationEntry represents a single entry in the citation chain.
type CitationEntry struct {
	UUID     string
	Summary  string // one-liner: turn number, tool name, target
	RawJSON  string // full formatted entry (shown when expanded)
	Expanded bool
}

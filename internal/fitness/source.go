package fitness

import "time"

// Source represents a tracked artifact in the source registry.
type Source struct {
	Name         string     `json:"name"`
	Origin       string     `json:"origin"`    // "user", "project:<name>", "plugin:<name>"
	Ownership    string     `json:"ownership"` // "mine", "not_mine", ""(unclassified)
	Approach     string     `json:"approach"`  // "iterate", "evaluate", "paused"
	SessionCount int        `json:"sessionCount"`
	HealthScore  float64    `json:"healthScore"` // 0-100, -1 for unclassified
	ClassifiedAt *time.Time `json:"classifiedAt,omitempty"`
}

// SourceGroup groups sources by origin for display.
type SourceGroup struct {
	Label     string   `json:"label"`  // display label ("User-level", "Project: foo")
	Origin    string   `json:"origin"` // raw origin key
	Sources   []Source `json:"sources"`
	Collapsed bool     `json:"-"` // UI state
}

// ChangeEntry records a historical change for rollback tracking.
type ChangeEntry struct {
	ID              string    `json:"id"`
	SourceName      string    `json:"sourceName"`
	SourceOrigin    string    `json:"sourceOrigin,omitempty"`
	ProposalID      string    `json:"proposalId"`
	Description     string    `json:"description"`
	Timestamp       time.Time `json:"timestamp"`
	Status          string    `json:"status"` // "approved", "rejected"
	PreviousContent string    `json:"previousContent,omitempty"` // for rollback
	FilePath        string    `json:"filePath"`
}



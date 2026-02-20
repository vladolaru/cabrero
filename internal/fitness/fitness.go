package fitness

import "time"

// Report is a fitness assessment for a third-party artifact (EVALUATE mode output).
type Report struct {
	ID            string          `json:"id"`
	SourceName    string          `json:"sourceName"`
	SourceOrigin  string          `json:"sourceOrigin"`  // "user", "project:<name>", "plugin:<name>"
	Ownership     string          `json:"ownership"`     // "mine" or "not_mine"
	ObservedCount int             `json:"observedCount"` // number of sessions observed
	WindowDays    int             `json:"windowDays"`    // observation window
	Assessment    Assessment      `json:"assessment"`
	Verdict       string          `json:"verdict"` // plain-language summary
	Evidence      []EvidenceGroup `json:"evidence"`
	GeneratedAt   time.Time       `json:"generatedAt"`
}

// Assessment holds the three-bucket health breakdown.
type Assessment struct {
	Followed     BucketStat `json:"followed"`     // used correctly
	WorkedAround BucketStat `json:"workedAround"` // manually overridden
	Confused     BucketStat `json:"confused"`      // caused errors/retries
}

// BucketStat holds count and percentage for one bucket.
type BucketStat struct {
	Count   int     `json:"count"`
	Percent float64 `json:"percent"` // 0-100
}

// EvidenceGroup groups session evidence by category.
type EvidenceGroup struct {
	Category string          `json:"category"` // "followed", "worked_around", "confused"
	Entries  []EvidenceEntry `json:"entries"`
	Expanded bool            `json:"-"` // UI state, not persisted
}

// EvidenceEntry is one session's evidence.
type EvidenceEntry struct {
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
	Summary   string    `json:"summary"`
	Detail    string    `json:"detail"`
}

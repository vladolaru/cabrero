// Package patterns provides cross-session pattern detection for recurring
// friction and errors across sessions in the same project.
package patterns

// AggregatorOutput is the result of cross-session pattern analysis.
type AggregatorOutput struct {
	ProjectSlug     string             `json:"projectSlug"`
	SessionsScanned int                `json:"sessionsScanned"`
	WindowDays      int                `json:"windowDays"`
	Patterns        []RecurringPattern `json:"patterns"`
}

// RecurringPattern describes a friction or error pattern recurring across sessions.
type RecurringPattern struct {
	Type         string   `json:"type"`                   // "correction_pattern" | "error_prone_sequence"
	ToolName     string   `json:"toolName"`
	Description  string   `json:"description"`
	SessionCount int      `json:"sessionCount"`
	SessionIDs   []string `json:"sessionIds"`
	ErrorSnippet string   `json:"errorSnippet,omitempty"`
	ErrorRate    float64  `json:"errorRate,omitempty"`
	BaselineRate float64  `json:"baselineRate,omitempty"`
}

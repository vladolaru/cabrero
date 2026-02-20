package fitness

import (
	"sort"
	"strings"
	"time"
)

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
	ProposalID      string    `json:"proposalId"`
	Description     string    `json:"description"`
	Timestamp       time.Time `json:"timestamp"`
	Status          string    `json:"status"` // "approved", "rejected"
	PreviousContent string    `json:"previousContent,omitempty"` // for rollback
	FilePath        string    `json:"filePath"`
}

// originOrder returns a sort key for origin-based group ordering.
// "user" first, then "project:*", then "plugin:*".
func originOrder(origin string) int {
	switch {
	case origin == "user":
		return 0
	case strings.HasPrefix(origin, "project:"):
		return 1
	case strings.HasPrefix(origin, "plugin:"):
		return 2
	default:
		return 3
	}
}

// originLabel returns a human-readable label for an origin key.
func originLabel(origin string) string {
	switch {
	case origin == "user":
		return "User-level"
	case strings.HasPrefix(origin, "project:"):
		return "Project: " + strings.TrimPrefix(origin, "project:")
	case strings.HasPrefix(origin, "plugin:"):
		return "Plugin: " + strings.TrimPrefix(origin, "plugin:")
	default:
		return origin
	}
}

// ListSourceGroups returns sources organized into groups by origin.
// Groups by origin, unclassified sources at the bottom.
func ListSourceGroups(sources []Source) []SourceGroup {
	// Separate classified and unclassified sources.
	classified := make(map[string][]Source)
	var unclassified []Source

	for _, s := range sources {
		if s.Ownership == "" {
			unclassified = append(unclassified, s)
			continue
		}
		classified[s.Origin] = append(classified[s.Origin], s)
	}

	// Collect unique origins and sort them.
	origins := make([]string, 0, len(classified))
	for o := range classified {
		origins = append(origins, o)
	}
	sort.Slice(origins, func(i, j int) bool {
		oi, oj := originOrder(origins[i]), originOrder(origins[j])
		if oi != oj {
			return oi < oj
		}
		return origins[i] < origins[j]
	})

	// Build groups.
	groups := make([]SourceGroup, 0, len(origins)+1)
	for _, o := range origins {
		groups = append(groups, SourceGroup{
			Label:   originLabel(o),
			Origin:  o,
			Sources: classified[o],
		})
	}

	// Append unclassified group last, if any.
	if len(unclassified) > 0 {
		groups = append(groups, SourceGroup{
			Label:   "\u26a0 Unclassified",
			Origin:  "",
			Sources: unclassified,
		})
	}

	return groups
}


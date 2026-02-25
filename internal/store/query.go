package store

import (
	"slices"
	"strings"
	"time"
)

// SessionFilter controls which sessions QuerySessions returns.
type SessionFilter struct {
	Since    time.Time // zero value = no lower bound
	Until    time.Time // zero value = no upper bound
	Project  string    // substring match (empty = all)
	Statuses []string  // e.g. ["pending"] or ["pending", "error"]; empty = all
}

// QuerySessions returns sessions matching the filter, sorted oldest-first.
func QuerySessions(filter SessionFilter) ([]Metadata, error) {
	all, err := ListSessions() // newest-first
	if err != nil {
		return nil, err
	}

	statusSet := make(map[string]bool, len(filter.Statuses))
	for _, s := range filter.Statuses {
		statusSet[s] = true
	}

	var matched []Metadata
	for _, m := range all {
		if len(statusSet) > 0 && !statusSet[m.Status] {
			continue
		}

		ts, err := time.Parse(time.RFC3339, m.Timestamp)
		if err != nil {
			continue
		}
		if !filter.Since.IsZero() && ts.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && ts.After(filter.Until) {
			continue
		}

		if filter.Project != "" && !strings.Contains(m.Project, filter.Project) {
			continue
		}

		matched = append(matched, m)
	}

	// ListSessions returns newest-first; reverse for oldest-first.
	slices.Reverse(matched)

	return matched, nil
}

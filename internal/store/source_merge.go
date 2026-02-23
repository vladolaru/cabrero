package store

import (
	"sort"
	"strings"

	"github.com/vladolaru/cabrero/internal/fitness"
)

// MergeSources combines persisted sources (with user classifications) and
// discovered sources (with session counts). Classification is always preserved
// from persisted data; session counts are updated from discovery.
func MergeSources(persisted, discovered []fitness.Source) []fitness.Source {
	// Index persisted by name for O(1) lookup.
	byName := make(map[string]*fitness.Source, len(persisted))
	result := make([]fitness.Source, len(persisted))
	for i, s := range persisted {
		result[i] = s
		byName[s.Name] = &result[i]
	}

	// Merge discovered sources.
	for _, d := range discovered {
		if existing, ok := byName[d.Name]; ok {
			// Update session count and origin (discovery has fresher data).
			existing.SessionCount = d.SessionCount
			if existing.Origin == "" {
				existing.Origin = d.Origin
			}
		} else {
			// New source — add as unclassified.
			result = append(result, d)
			byName[d.Name] = &result[len(result)-1]
		}
	}

	return result
}

// LoadAndMergeSources reads persisted sources, runs discovery, merges,
// and writes the result back. Returns the merged sources.
func LoadAndMergeSources() ([]fitness.Source, error) {
	persisted, err := ReadSources()
	if err != nil {
		persisted = []fitness.Source{}
	}

	discovered, err := DiscoverSourcesFromEvaluations()
	if err != nil {
		discovered = []fitness.Source{}
	}

	merged := MergeSources(persisted, discovered)

	// Persist the merged result (non-fatal if this fails).
	_ = WriteSources(merged)

	return merged, nil
}

// GroupSources organizes a flat source slice into display groups.
// Unclassified sources (ownership=="") are placed in a separate group first.
// Remaining sources are grouped by origin.
func GroupSources(sources []fitness.Source) []fitness.SourceGroup {
	var unclassified []fitness.Source
	byOrigin := map[string][]fitness.Source{}

	for _, s := range sources {
		if s.Ownership == "" {
			unclassified = append(unclassified, s)
		} else {
			byOrigin[s.Origin] = append(byOrigin[s.Origin], s)
		}
	}

	var groups []fitness.SourceGroup

	// Unclassified first (only if non-empty).
	if len(unclassified) > 0 {
		groups = append(groups, fitness.SourceGroup{
			Label:   "Unclassified",
			Origin:  "",
			Sources: unclassified,
		})
	}

	// Sort origin keys for stable output.
	origins := make([]string, 0, len(byOrigin))
	for o := range byOrigin {
		origins = append(origins, o)
	}
	sort.Strings(origins)

	for _, o := range origins {
		groups = append(groups, fitness.SourceGroup{
			Label:   originLabel(o),
			Origin:  o,
			Sources: byOrigin[o],
		})
	}

	return groups
}

// originLabel converts an origin string to a display label.
func originLabel(origin string) string {
	switch {
	case origin == "user":
		return "User-level"
	case strings.HasPrefix(origin, "project:"):
		return "Project: " + origin[len("project:"):]
	case strings.HasPrefix(origin, "plugin:"):
		return "Plugin: " + origin[len("plugin:"):]
	default:
		return origin
	}
}

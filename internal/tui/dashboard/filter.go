package dashboard

import (
	"strings"

	"charm.land/bubbles/v2/list"
)

// dashboardFilter implements list.FilterFunc with support for:
//   - "type:<val>"   — match items whose TypeName contains val
//   - "target:<val>" — match items whose Target contains val
//   - free text      — match anywhere in the FilterValue string
//
// All matching is case-insensitive. FilterValue() must produce the tagged
// format "type:<T> target:<U> confidence:<C>" for prefix matching to work.
func dashboardFilter(term string, targets []string) []list.Rank {
	lower := strings.ToLower(strings.TrimSpace(term))

	if lower == "" {
		ranks := make([]list.Rank, len(targets))
		for i := range ranks {
			ranks[i] = list.Rank{Index: i}
		}
		return ranks
	}

	var matchFn func(target string) bool

	if val, ok := strings.CutPrefix(lower, "type:"); ok {
		matchFn = func(target string) bool {
			return strings.Contains(extractTag(target, "type:"), val)
		}
	} else if val, ok := strings.CutPrefix(lower, "target:"); ok {
		matchFn = func(target string) bool {
			return strings.Contains(extractTag(target, "target:"), val)
		}
	} else {
		matchFn = func(target string) bool {
			return strings.Contains(strings.ToLower(target), lower)
		}
	}

	var ranks []list.Rank
	for i, t := range targets {
		if matchFn(strings.ToLower(t)) {
			ranks = append(ranks, list.Rank{Index: i})
		}
	}
	return ranks
}

// extractTag returns the value of a tagged field in a FilterValue string.
// e.g. extractTag("type:foo target:bar", "target:") → "bar"
func extractTag(s, tag string) string {
	idx := strings.Index(s, tag)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(tag):]
	if space := strings.Index(rest, " "); space >= 0 {
		return rest[:space]
	}
	return rest
}

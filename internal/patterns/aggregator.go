package patterns

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/store"
)

// Tuning constants.
const (
	windowDays      = 30
	maxSessions     = 20
	minSessionCount = 3
	maxPatterns     = 5
	maxSnippetLen   = 100
	maxSessionIDs   = 5
)

// normalizeRe strips numbers, UUIDs, and collapses whitespace for snippet grouping.
var normalizeRe = regexp.MustCompile(`[0-9a-f]{8,}|[0-9]+`)

// Aggregate performs cross-session pattern detection for a project.
// Returns nil if insufficient data (fewer than minSessionCount digests).
func Aggregate(currentSessionID, projectSlug string) (*AggregatorOutput, error) {
	if projectSlug == "" {
		return nil, nil
	}

	sessions, err := store.ListSessions()
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().AddDate(0, 0, -windowDays)

	// Filter: same project, exclude current session, within time window.
	var candidates []store.Metadata
	for _, s := range sessions {
		if s.Project != projectSlug || s.SessionID == currentSessionID {
			continue
		}
		ts, err := time.Parse(time.RFC3339, s.Timestamp)
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			continue
		}
		candidates = append(candidates, s)
	}

	// Limit to most recent sessions (already sorted desc by ListSessions).
	if len(candidates) > maxSessions {
		candidates = candidates[:maxSessions]
	}

	// Load digests.
	type digestWithID struct {
		sessionID string
		digest    *parser.Digest
	}
	var digests []digestWithID
	for _, s := range candidates {
		d, err := parser.ReadDigest(s.SessionID)
		if err != nil {
			continue // skip sessions without digests
		}
		digests = append(digests, digestWithID{sessionID: s.SessionID, digest: d})
	}

	if len(digests) < minSessionCount {
		return nil, nil
	}

	output := &AggregatorOutput{
		ProjectSlug:     projectSlug,
		SessionsScanned: len(digests),
		WindowDays:      windowDays,
	}

	// Compute project baseline: totalErrors / totalCalls per tool.
	type toolStats struct {
		totalCalls  int
		totalErrors int
		sessions    map[string]bool
	}
	baseline := make(map[string]*toolStats)

	for _, dw := range digests {
		for toolName, detail := range dw.digest.ToolCalls.Summary {
			stats, ok := baseline[toolName]
			if !ok {
				stats = &toolStats{sessions: make(map[string]bool)}
				baseline[toolName] = stats
			}
			stats.totalCalls += detail.Count
			stats.totalErrors += detail.ErrorCount
			stats.sessions[dw.sessionID] = true
		}
	}

	// Detect correction patterns: group ErrorEntry items by (toolName, normalizedSnippet).
	type errorKey struct {
		toolName string
		snippet  string
	}
	errorGroups := make(map[errorKey]map[string]string) // key → sessionID → first snippet
	for _, dw := range digests {
		for _, e := range dw.digest.Errors {
			tn := ""
			if e.ToolName != nil {
				tn = *e.ToolName
			}
			if tn == "" {
				continue
			}
			normalized := normalizeSnippet(e.Snippet)
			key := errorKey{toolName: tn, snippet: normalized}
			if errorGroups[key] == nil {
				errorGroups[key] = make(map[string]string)
			}
			if _, exists := errorGroups[key][dw.sessionID]; !exists {
				errorGroups[key][dw.sessionID] = e.Snippet // keep first raw snippet
			}
		}
	}

	for key, sessionMap := range errorGroups {
		if len(sessionMap) < minSessionCount {
			continue
		}
		ids := sortedKeys(sessionMap)
		snippet := ""
		for _, id := range ids {
			snippet = sessionMap[id]
			break
		}
		output.Patterns = append(output.Patterns, RecurringPattern{
			Type:         "correction_pattern",
			ToolName:     key.toolName,
			Description:  "same error recurring across sessions: " + truncateSnippet(key.snippet, maxSnippetLen),
			SessionCount: len(sessionMap),
			SessionIDs:   capSlice(ids, maxSessionIDs),
			ErrorSnippet: truncateSnippet(snippet, maxSnippetLen),
		})
	}

	// Detect error-prone sequences: tools with error rate > 2× baseline, in 3+ sessions with ≥3 errors.
	for toolName, stats := range baseline {
		if stats.totalCalls == 0 || stats.totalErrors < 3 {
			continue
		}
		if len(stats.sessions) < minSessionCount {
			continue
		}

		errorRate := float64(stats.totalErrors) / float64(stats.totalCalls)

		// Compute global baseline (all tools combined).
		var globalCalls, globalErrors int
		for _, s := range baseline {
			globalCalls += s.totalCalls
			globalErrors += s.totalErrors
		}
		globalRate := 0.0
		if globalCalls > 0 {
			globalRate = float64(globalErrors) / float64(globalCalls)
		}

		if globalRate > 0 && errorRate > 2*globalRate {
			ids := sortedKeys(stats.sessions)
			output.Patterns = append(output.Patterns, RecurringPattern{
				Type:         "error_prone_sequence",
				ToolName:     toolName,
				Description:  toolName + " has elevated error rate across sessions",
				SessionCount: len(stats.sessions),
				SessionIDs:   capSlice(ids, maxSessionIDs),
				ErrorRate:    errorRate,
				BaselineRate: globalRate,
			})
		}
	}

	// Error anchoring: only keep patterns with associated errors or friction.
	// Since both detection methods already require errors, this is inherently satisfied.

	// Sort by session count descending, limit to top N.
	sort.Slice(output.Patterns, func(i, j int) bool {
		return output.Patterns[i].SessionCount > output.Patterns[j].SessionCount
	})
	if len(output.Patterns) > maxPatterns {
		output.Patterns = output.Patterns[:maxPatterns]
	}

	return output, nil
}

func normalizeSnippet(s string) string {
	s = strings.ToLower(s)
	s = normalizeRe.ReplaceAllString(s, "N")
	// Collapse whitespace.
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func truncateSnippet(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func capSlice(s []string, max int) []string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

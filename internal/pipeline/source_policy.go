package pipeline

import (
	"os"

	"github.com/vladolaru/cabrero/internal/store"
)

// SourceGateResult describes whether a session is allowed to proceed to the
// evaluator, or blocked by source-governance policy.
type SourceGateResult struct {
	Allowed    bool
	Reason     string // "", "unclassified_source", "paused_source"
	SourceName string // name of the blocking source (for logging)
}

// CheckSourcePolicy derives the sources touched by a classifier output,
// resolves their classification from sources.json, and returns a gate decision.
//
// Policy: if ANY touched source is unclassified (ownership=="") or paused
// (approach=="paused"), the evaluator is skipped.
//
// Fail-open: if sources.json can't be read or has no entries, allow.
func CheckSourcePolicy(co *ClassifierOutput) SourceGateResult {
	if co == nil {
		return SourceGateResult{Allowed: true}
	}

	// Derive touched source identities from classifier signals.
	type sourceID struct{ Name, Origin string }
	touched := make(map[sourceID]bool)

	for _, sig := range co.SkillSignals {
		name, origin := store.InferOrigin(sig.SkillName)
		touched[sourceID{name, origin}] = true
	}

	home, _ := os.UserHomeDir()
	for _, sig := range co.ClaudeMdSignals {
		name, origin := store.InferOriginFromPath(sig.Path, home)
		touched[sourceID{name, origin}] = true
	}

	if len(touched) == 0 {
		return SourceGateResult{Allowed: true}
	}

	// Load registered sources.
	sources, err := store.ReadSources()
	if err != nil || len(sources) == 0 {
		// Can't verify policy — fail open.
		return SourceGateResult{Allowed: true}
	}

	// Index by (name, origin) for O(1) lookup.
	type key struct{ Name, Origin string }
	byID := make(map[key]struct{ Ownership, Approach string }, len(sources))
	for _, s := range sources {
		byID[key{s.Name, s.Origin}] = struct{ Ownership, Approach string }{s.Ownership, s.Approach}
	}

	// Check each touched source against policy.
	for id := range touched {
		s, found := byID[key{id.Name, id.Origin}]
		if !found || s.Ownership == "" {
			return SourceGateResult{
				Allowed:    false,
				Reason:     GateReasonUnclassified,
				SourceName: id.Name,
			}
		}
		if s.Approach == "paused" {
			return SourceGateResult{
				Allowed:    false,
				Reason:     GateReasonPaused,
				SourceName: id.Name,
			}
		}
	}

	return SourceGateResult{Allowed: true}
}

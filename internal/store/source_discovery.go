package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/vladolaru/cabrero/internal/fitness"
)

// sourceKey uniquely identifies a discovered source.
type sourceKey struct {
	Name   string
	Origin string
}

// classifierSignals is a minimal projection of pipeline.ClassifierOutput
// containing only the fields needed for source discovery. Defined locally
// to avoid a circular import (store ← pipeline → store).
type classifierSignals struct {
	SkillSignals    []classifierSkillSignal   `json:"skillSignals"`
	ClaudeMdSignals []classifierClaudeMdSignal `json:"claudeMdSignals"`
}

type classifierSkillSignal struct {
	SkillName string `json:"skillName"`
}

type classifierClaudeMdSignal struct {
	Path string `json:"path"`
}

// DiscoverSourcesFromEvaluations scans classifier output files and extracts
// unique sources (skills and CLAUDE.md files) with session counts.
func DiscoverSourcesFromEvaluations() ([]fitness.Source, error) {
	evalDir := filepath.Join(Root(), "evaluations")
	entries, err := os.ReadDir(evalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []fitness.Source{}, nil
		}
		return nil, err
	}

	home, _ := os.UserHomeDir()

	// Track unique sources and their session counts.
	// Each classifier output represents one session's worth of signals.
	// A source seen multiple times within one classifier counts once per session.
	counts := map[sourceKey]int{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "-classifier.json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(evalDir, e.Name()))
		if err != nil {
			continue
		}

		var c classifierSignals
		if err := json.Unmarshal(data, &c); err != nil {
			continue
		}

		// Deduplicate within this session.
		seen := map[sourceKey]bool{}

		for _, sig := range c.SkillSignals {
			name, origin := InferOrigin(sig.SkillName)
			k := sourceKey{Name: name, Origin: origin}
			seen[k] = true
		}

		for _, sig := range c.ClaudeMdSignals {
			name, origin := InferOriginFromPath(sig.Path, home)
			k := sourceKey{Name: name, Origin: origin}
			seen[k] = true
		}

		for k := range seen {
			counts[k]++
		}
	}

	sources := make([]fitness.Source, 0, len(counts))
	for k, count := range counts {
		sources = append(sources, fitness.Source{
			Name:         k.Name,
			Origin:       k.Origin,
			SessionCount: count,
			HealthScore:  -1, // unscored
		})
	}

	return sources, nil
}

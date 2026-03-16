package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ignoreMu sync.Mutex

func ignoredProjectsPath() string {
	return filepath.Join(Root(), "ignored_projects.json")
}

// IgnoredPattern is a substring pattern for project slugs to ignore.
type IgnoredPattern struct {
	Pattern string    `json:"pattern"`
	AddedAt time.Time `json:"added_at"`
}

type ignoredProjectsFile struct {
	Patterns []IgnoredPattern `json:"patterns"`
}

// ReadIgnoredPatterns returns all ignored project patterns.
func ReadIgnoredPatterns() ([]IgnoredPattern, error) {
	ignoreMu.Lock()
	defer ignoreMu.Unlock()
	return readIgnoredPatterns()
}

func readIgnoredPatterns() ([]IgnoredPattern, error) {
	data, err := os.ReadFile(ignoredProjectsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f ignoredProjectsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing ignored_projects.json: %w", err)
	}
	return f.Patterns, nil
}

func writeIgnoredPatterns(patterns []IgnoredPattern) error {
	f := ignoredProjectsFile{Patterns: patterns}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(ignoredProjectsPath(), data, 0o644)
}

// AddIgnoredPattern adds a substring pattern. No-op if already present
// (case-insensitive match on pattern text).
func AddIgnoredPattern(pattern string) error {
	ignoreMu.Lock()
	defer ignoreMu.Unlock()

	patterns, err := readIgnoredPatterns()
	if err != nil {
		return err
	}
	lower := strings.ToLower(pattern)
	for _, p := range patterns {
		if strings.ToLower(p.Pattern) == lower {
			return nil // already exists
		}
	}
	patterns = append(patterns, IgnoredPattern{
		Pattern: pattern,
		AddedAt: time.Now().UTC(),
	})
	return writeIgnoredPatterns(patterns)
}

// RemoveIgnoredPattern removes a pattern by exact match (case-insensitive).
// Returns true if found and removed.
func RemoveIgnoredPattern(pattern string) (bool, error) {
	ignoreMu.Lock()
	defer ignoreMu.Unlock()

	patterns, err := readIgnoredPatterns()
	if err != nil {
		return false, err
	}
	lower := strings.ToLower(pattern)
	found := false
	kept := patterns[:0]
	for _, p := range patterns {
		if strings.ToLower(p.Pattern) == lower {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if !found {
		return false, nil
	}
	if err := writeIgnoredPatterns(kept); err != nil {
		return false, err
	}
	return true, nil
}

// IsProjectIgnored returns true if the project slug matches any ignored pattern
// (case-insensitive substring match).
func IsProjectIgnored(slug string) bool {
	if slug == "" {
		return false
	}
	ignoreMu.Lock()
	defer ignoreMu.Unlock()

	patterns, err := readIgnoredPatterns()
	if err != nil {
		return false
	}
	lower := strings.ToLower(slug)
	for _, p := range patterns {
		if strings.Contains(lower, strings.ToLower(p.Pattern)) {
			return true
		}
	}
	return false
}

// CountIgnoredSessions returns the number of existing sessions whose project
// matches an ignored pattern.
func CountIgnoredSessions() int {
	sessions, err := ListSessions()
	if err != nil {
		return 0
	}
	count := 0
	for _, s := range sessions {
		if IsProjectIgnored(s.Project) {
			count++
		}
	}
	return count
}

// CleanIgnoredSessions removes raw session directories for sessions whose
// project matches an ignored pattern. Also removes matching entries from the
// blocklist. Returns the number of sessions removed.
func CleanIgnoredSessions() (int, error) {
	sessions, err := ListSessions()
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, s := range sessions {
		if !IsProjectIgnored(s.Project) {
			continue
		}
		dir := RawDir(s.SessionID)
		if err := os.RemoveAll(dir); err != nil {
			continue
		}
		// Best-effort: remove from blocklist if present.
		UnblockSession(s.SessionID) //nolint:errcheck
		removed++
	}
	return removed, nil
}

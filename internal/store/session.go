package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Metadata stored alongside each raw session backup.
type Metadata struct {
	SessionID      string `json:"session_id"`
	Timestamp      string `json:"timestamp"`
	CaptureTrigger string `json:"capture_trigger"`
	CCVersion      string `json:"cc_version,omitempty"`
	Status         string `json:"status"`              // "pending" or "processed"
	Project        string `json:"project,omitempty"`    // CC project slug (parent dir name)
}

// SessionExists returns true if a raw backup already exists for the session.
func SessionExists(sessionID string) bool {
	meta := filepath.Join(RawDir(sessionID), "metadata.json")
	_, err := os.Stat(meta)
	return err == nil
}

// WriteSession copies a transcript JSONL file into the store and writes metadata.
// The timestamp should reflect the original session time, not the import time.
func WriteSession(sessionID, transcriptSrc, trigger, ccVersion string, ts time.Time, project string) error {
	dir := RawDir(sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating session dir: %w", err)
	}

	// Copy transcript.
	data, err := os.ReadFile(transcriptSrc)
	if err != nil {
		return fmt.Errorf("reading transcript: %w", err)
	}
	dst := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("writing transcript: %w", err)
	}

	meta := Metadata{
		SessionID:      sessionID,
		Timestamp:      ts.UTC().Format(time.RFC3339),
		CaptureTrigger: trigger,
		CCVersion:      ccVersion,
		Status:         "pending",
		Project:        project,
	}
	return WriteMetadata(dir, meta)
}

// WriteMetadata writes a Metadata struct to metadata.json in the given directory.
func WriteMetadata(dir string, meta Metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644)
}

// ReadMetadata reads metadata.json from a session's raw directory.
func ReadMetadata(sessionID string) (Metadata, error) {
	path := filepath.Join(RawDir(sessionID), "metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("parsing metadata for %s: %w", sessionID, err)
	}
	return meta, nil
}

// ListSessions returns metadata for all sessions in the store, sorted by
// timestamp descending (most recent first).
func ListSessions() ([]Metadata, error) {
	rawDir := filepath.Join(Root(), "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []Metadata
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := ReadMetadata(e.Name())
		if err != nil {
			continue
		}
		sessions = append(sessions, meta)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp > sessions[j].Timestamp
	})
	return sessions, nil
}

// ProjectDisplayName converts a CC project slug into a human-readable short
// name by stripping the home directory prefix portion.
//
// Example: "-Users-vladolaru-Work-a8c-woocommerce-payments" → "Work/a8c/woocommerce-payments"
//          "-Users-vladolaru--claude" → ".claude"
//          "-private-tmp" → "/private/tmp"
func ProjectDisplayName(slug string) string {
	if slug == "" {
		return ""
	}

	// Convert slug back to a path: leading dash is /, dashes are /.
	// Double dashes (--) encode a dot-prefixed component (e.g. /.claude → --claude).
	path := "/" + strings.TrimPrefix(slug, "-")

	// Replace -- with a placeholder, then convert single - to /, then restore dots.
	path = strings.ReplaceAll(path, "--", "\x00DOT\x00")
	path = strings.ReplaceAll(path, "-", "/")
	path = strings.ReplaceAll(path, "\x00DOT\x00", "/.")

	// Strip home directory prefix to shorten.
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home+"/") {
		path = path[len(home)+1:]
	} else if strings.HasPrefix(path, home) && path == home {
		path = "~"
	}

	return path
}

// ListProjects returns a sorted list of unique project slugs from the store.
func ListProjects() ([]string, error) {
	sessions, err := ListSessions()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var projects []string
	for _, s := range sessions {
		if s.Project != "" && !seen[s.Project] {
			seen[s.Project] = true
			projects = append(projects, s.Project)
		}
	}
	sort.Strings(projects)
	return projects, nil
}

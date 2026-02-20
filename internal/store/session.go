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
	Status         string `json:"status"`              // "queued", "imported", "processed", or "error"
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
		Status:         "imported",
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

// MarkProcessed sets a session's status to "processed".
func MarkProcessed(sessionID string) error {
	meta, err := ReadMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("reading metadata for %s: %w", sessionID, err)
	}
	meta.Status = "processed"
	return WriteMetadata(RawDir(sessionID), meta)
}

// MarkQueued sets a session's status to "queued" so the daemon will process it.
func MarkQueued(sessionID string) error {
	meta, err := ReadMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("reading metadata for %s: %w", sessionID, err)
	}
	meta.Status = "queued"
	return WriteMetadata(RawDir(sessionID), meta)
}

// MarkError sets a session's status to "error".
func MarkError(sessionID string) error {
	meta, err := ReadMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("reading metadata for %s: %w", sessionID, err)
	}
	meta.Status = "error"
	return WriteMetadata(RawDir(sessionID), meta)
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

// ProjectDisplayName returns a short, recognizable form of a CC project slug.
//
// CC encodes project paths by replacing both '/' and '.' with '-', making the
// slug non-reversible (a '-' could be any of the three). Instead of guessing,
// we strip the home directory prefix from the slug to shorten it while keeping
// the original encoding intact.
//
// Example: "-Users-vladolaru-Work-a8c-woocommerce-payments" → "Work-a8c-woocommerce-payments"
//          "-Users-vladolaru--claude" → "-claude"
//          "-private-tmp" → "/private-tmp"
func ProjectDisplayName(slug string) string {
	if slug == "" {
		return ""
	}

	// Build the home directory prefix as CC would encode it.
	home, err := os.UserHomeDir()
	if err != nil {
		return slug
	}
	homeSlug := strings.ReplaceAll(home, "/", "-")
	homeSlug = strings.ReplaceAll(homeSlug, ".", "-")

	// Strip the home prefix. What remains is recognizable as the project
	// path relative to ~, with the original hyphens preserved.
	if strings.HasPrefix(slug, homeSlug+"-") {
		return slug[len(homeSlug)+1:]
	}
	if slug == homeSlug {
		return "~"
	}

	return slug
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

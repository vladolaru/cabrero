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

// Session status values.
const (
	StatusQueued         = "queued"
	StatusImported       = "imported"
	StatusProcessed      = "processed"
	StatusError          = "error"
	StatusCaptureFailed  = "capture_failed"
)

// Metadata stored alongside each raw session backup.
type Metadata struct {
	SessionID      string `json:"session_id"`
	Timestamp      string `json:"timestamp"`
	CaptureTrigger string `json:"capture_trigger"`
	CCVersion      string `json:"cc_version,omitempty"`
	Status         string `json:"status"`              // "queued", "imported", "processed", "error", or "capture_failed"
	Project        string `json:"project,omitempty"`    // CC project slug (parent dir name)
	WorkDir        string `json:"work_dir,omitempty"`   // working directory from CC hook payload
}

// SessionExists returns true if a raw backup already exists for the session.
func SessionExists(sessionID string) bool {
	meta := filepath.Join(RawDir(sessionID), "metadata.json")
	_, err := os.Stat(meta)
	return err == nil
}

// TranscriptExists returns true if a transcript.jsonl file exists for the session.
func TranscriptExists(sessionID string) bool {
	path := filepath.Join(RawDir(sessionID), "transcript.jsonl")
	_, err := os.Stat(path)
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
		Status:         StatusImported,
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
	return AtomicWrite(filepath.Join(dir, "metadata.json"), data, 0o644)
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
	// Backfill: derive project slug from WorkDir when Project is missing.
	if meta.Project == "" && meta.WorkDir != "" {
		meta.Project = ProjectSlugFromPath(meta.WorkDir)
	}
	return meta, nil
}

// ProjectSlugFromPath converts an absolute filesystem path into a CC-style
// project slug by replacing '/' and '.' with '-'.
// Example: "/Users/vladolaru/Work/a8c/cabrero" → "-Users-vladolaru-Work-a8c-cabrero"
func ProjectSlugFromPath(path string) string {
	s := strings.ReplaceAll(path, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	return s
}

// MarkProcessed sets a session's status to "processed".
func MarkProcessed(sessionID string) error {
	meta, err := ReadMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("reading metadata for %s: %w", sessionID, err)
	}
	meta.Status = StatusProcessed
	return WriteMetadata(RawDir(sessionID), meta)
}

// MarkQueued sets a session's status to "queued" so the daemon will process it.
func MarkQueued(sessionID string) error {
	meta, err := ReadMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("reading metadata for %s: %w", sessionID, err)
	}
	meta.Status = StatusQueued
	return WriteMetadata(RawDir(sessionID), meta)
}

// MarkError sets a session's status to "error".
func MarkError(sessionID string) error {
	meta, err := ReadMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("reading metadata for %s: %w", sessionID, err)
	}
	meta.Status = StatusError
	return WriteMetadata(RawDir(sessionID), meta)
}

// MarkCaptureFailed sets a session's status to "capture_failed".
// Used for sessions where the transcript copy failed during hook capture
// (no transcript available, unrecoverable).
func MarkCaptureFailed(sessionID string) error {
	meta, err := ReadMetadata(sessionID)
	if err != nil {
		return fmt.Errorf("reading metadata for %s: %w", sessionID, err)
	}
	meta.Status = StatusCaptureFailed
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

// ShortSessionID returns the first 8 characters of a session ID (the first
// UUID segment). Use this for display in logs, CLI output, and TUI views.
func ShortSessionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
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


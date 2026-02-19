package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Metadata stored alongside each raw session backup.
type Metadata struct {
	SessionID      string `json:"session_id"`
	Timestamp      string `json:"timestamp"`
	CaptureTrigger string `json:"capture_trigger"`
	CCVersion      string `json:"cc_version,omitempty"`
	Status         string `json:"status"` // "pending" or "processed"
}

// SessionExists returns true if a raw backup already exists for the session.
func SessionExists(sessionID string) bool {
	meta := filepath.Join(RawDir(sessionID), "metadata.json")
	_, err := os.Stat(meta)
	return err == nil
}

// WriteSession copies a transcript JSONL file into the store and writes metadata.
func WriteSession(sessionID, transcriptSrc, trigger, ccVersion string) error {
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

	// Write metadata.
	meta := Metadata{
		SessionID:      sessionID,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CaptureTrigger: trigger,
		CCVersion:      ccVersion,
		Status:         "pending",
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
			// Skip sessions with unreadable metadata.
			continue
		}
		sessions = append(sessions, meta)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp > sessions[j].Timestamp
	})
	return sessions, nil
}

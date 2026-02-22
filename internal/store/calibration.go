package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CalibrationEntry represents a single tagged session in the calibration set.
type CalibrationEntry struct {
	SessionID string    `json:"sessionId"`
	Label     string    `json:"label"`          // "approve" or "reject"
	Note      string    `json:"note,omitempty"`
	TaggedAt  time.Time `json:"taggedAt"`
}

// calibrationFile is the on-disk JSON envelope.
type calibrationFile struct {
	Entries []CalibrationEntry `json:"entries"`
}

var calibrationMu sync.Mutex

func calibrationPath() string {
	return filepath.Join(Root(), "calibration.json")
}

// readCalibration reads the calibration file from disk.
// Returns an empty slice if the file doesn't exist.
func readCalibration() ([]CalibrationEntry, error) {
	data, err := os.ReadFile(calibrationPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cf calibrationFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing calibration.json: %w", err)
	}
	return cf.Entries, nil
}

// writeCalibration writes the calibration entries to disk atomically.
func writeCalibration(entries []CalibrationEntry) error {
	cf := calibrationFile{Entries: entries}
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(calibrationPath(), data, 0o644)
}

// AddCalibrationEntry validates and appends an entry to the calibration set.
// It auto-sets TaggedAt if zero, rejects invalid labels, and prevents
// duplicate session IDs.
func AddCalibrationEntry(entry CalibrationEntry) error {
	if entry.Label != "approve" && entry.Label != "reject" {
		return fmt.Errorf("invalid label %q: must be \"approve\" or \"reject\"", entry.Label)
	}

	calibrationMu.Lock()
	defer calibrationMu.Unlock()

	entries, err := readCalibration()
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.SessionID == entry.SessionID {
			return fmt.Errorf("session %q already in calibration set", entry.SessionID)
		}
	}

	if entry.TaggedAt.IsZero() {
		entry.TaggedAt = time.Now().UTC()
	}

	entries = append(entries, entry)
	return writeCalibration(entries)
}

// RemoveCalibrationEntry removes the entry with the given session ID.
// Returns an error if no such entry exists.
func RemoveCalibrationEntry(sessionID string) error {
	calibrationMu.Lock()
	defer calibrationMu.Unlock()

	entries, err := readCalibration()
	if err != nil {
		return err
	}

	found := false
	var remaining []CalibrationEntry
	for _, e := range entries {
		if e.SessionID == sessionID {
			found = true
			continue
		}
		remaining = append(remaining, e)
	}

	if !found {
		return fmt.Errorf("session %q not in calibration set", sessionID)
	}

	return writeCalibration(remaining)
}

// ListCalibrationEntries reads and returns all calibration entries.
// Returns an empty slice (not nil) if the file doesn't exist.
func ListCalibrationEntries() ([]CalibrationEntry, error) {
	calibrationMu.Lock()
	defer calibrationMu.Unlock()

	entries, err := readCalibration()
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []CalibrationEntry{}
	}
	return entries, nil
}

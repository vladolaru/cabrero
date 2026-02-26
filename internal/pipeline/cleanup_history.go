package pipeline

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// CleanupRecord captures the full diagnostic context of a single cleanup run.
type CleanupRecord struct {
	Timestamp       time.Time         `json:"timestamp"`
	DurationNs      int64             `json:"duration_ns"`
	ProposalsBefore int               `json:"proposals_before"`
	ProposalsAfter  int               `json:"proposals_after"`
	Decisions       []CuratorDecision `json:"decisions"`
	CuratorUsage    []InvocationUsage `json:"curator_usage,omitempty"` // one per Sonnet call
	CheckUsage      *InvocationUsage  `json:"check_usage,omitempty"`   // the Haiku batch call
	Error           string            `json:"error,omitempty"`
}

// Duration returns the cleanup run duration.
func (r CleanupRecord) Duration() time.Duration {
	return time.Duration(r.DurationNs)
}

var cleanupHistoryMu sync.Mutex

// cleanupHistoryPath returns the path to the cleanup history JSONL file.
// Declared as a variable so tests can override it.
var cleanupHistoryPath = func() string {
	return filepath.Join(store.Root(), "cleanup_history.jsonl")
}

// AppendCleanupHistory appends a single cleanup record to the JSONL file.
// Thread-safe. Best-effort — callers should not fail the cleanup run on error.
func AppendCleanupHistory(rec CleanupRecord) error {
	cleanupHistoryMu.Lock()
	defer cleanupHistoryMu.Unlock()

	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	f, err := os.OpenFile(cleanupHistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// ReadCleanupHistory reads all cleanup records from the JSONL file.
// Returns nil, nil for a missing or empty file. Malformed lines are skipped.
func ReadCleanupHistory() ([]CleanupRecord, error) {
	cleanupHistoryMu.Lock()
	defer cleanupHistoryMu.Unlock()
	return readCleanupHistoryFrom(cleanupHistoryPath())
}

func readCleanupHistoryFrom(path string) ([]CleanupRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var records []CleanupRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec CleanupRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue // skip malformed lines
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return records, err
	}
	return records, nil
}

// RotateCleanupHistory removes records older than maxAge from the history file.
// Returns the count of removed records.
func RotateCleanupHistory(maxAge time.Duration) (int, error) {
	cleanupHistoryMu.Lock()
	defer cleanupHistoryMu.Unlock()

	path := cleanupHistoryPath()
	records, err := readCleanupHistoryFrom(path)
	if err != nil || len(records) == 0 {
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	var kept []CleanupRecord
	removed := 0
	for _, rec := range records {
		if rec.Timestamp.Before(cutoff) {
			removed++
		} else {
			kept = append(kept, rec)
		}
	}
	if removed == 0 {
		return 0, nil
	}

	var data []byte
	for _, rec := range kept {
		line, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := store.AtomicWrite(path, data, 0o644); err != nil {
		return 0, err
	}
	return removed, nil
}

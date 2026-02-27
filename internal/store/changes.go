package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/vladolaru/cabrero/internal/fitness"
)

const changesFile = "changes.jsonl"

// changesPath returns the full path to the changes JSONL file.
func changesPath() string {
	return filepath.Join(Root(), changesFile)
}

// AppendChange appends a change entry to the JSONL file.
func AppendChange(entry fitness.ChangeEntry) error {
	p := changesPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// ChangesBySource returns all change entries for a given source name,
// in chronological order (oldest first). Returns empty slice if no file exists.
func ChangesBySource(sourceName string) ([]fitness.ChangeEntry, error) {
	entries, err := readAllChanges()
	if err != nil {
		return nil, err
	}
	var result []fitness.ChangeEntry
	for _, e := range entries {
		if e.SourceName == sourceName {
			result = append(result, e)
		}
	}
	return result, nil
}

// GetChange returns a change entry by ID, or nil if not found.
func GetChange(id string) (*fitness.ChangeEntry, error) {
	entries, err := readAllChanges()
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].ID == id {
			return &entries[i], nil
		}
	}
	return nil, nil
}

// readAllChanges reads all entries from the JSONL file.
func readAllChanges() ([]fitness.ChangeEntry, error) {
	p := changesPath()
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []fitness.ChangeEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e fitness.ChangeEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

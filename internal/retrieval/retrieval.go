// Package retrieval provides low-level access to raw session transcript entries.
// All functions read from ~/.cabrero/raw/{sessionID}/ and return raw bytes.
// Callers are responsible for parsing what they need.
package retrieval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vladolaru/cabrero/internal/store"
)

// maxScanBuffer is 10 MB to handle large JSONL lines (tool results can be 100KB+).
const maxScanBuffer = 10 * 1024 * 1024

// uuidField is the minimal struct for extracting just the uuid from a JSONL line.
type uuidField struct {
	UUID string `json:"uuid"`
}

// GetEntry returns the raw JSONL line for a specific UUID in a session.
func GetEntry(sessionID, uuid string) ([]byte, error) {
	path := transcriptPath(sessionID)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening transcript: %w", err)
	}
	defer f.Close()

	scanner := newScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		var entry uuidField
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.UUID == uuid {
			// Return a copy since scanner reuses the buffer.
			result := make([]byte, len(line))
			copy(result, line)
			return result, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning transcript: %w", err)
	}

	return nil, fmt.Errorf("uuid %s not found in session %s", uuid, sessionID)
}

// GetTurns returns ordered raw JSONL lines for a list of UUIDs.
// Single pass through the file, collecting matches. Returns in the order
// of the input UUID list. Missing UUIDs produce an error.
func GetTurns(sessionID string, uuids []string) ([][]byte, error) {
	if len(uuids) == 0 {
		return nil, nil
	}

	path := transcriptPath(sessionID)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening transcript: %w", err)
	}
	defer f.Close()

	// Build lookup map.
	wanted := make(map[string]int, len(uuids))
	for i, u := range uuids {
		wanted[u] = i
	}

	results := make([][]byte, len(uuids))
	found := 0

	scanner := newScanner(f)
	for scanner.Scan() {
		if found == len(uuids) {
			break
		}

		line := scanner.Bytes()
		var entry uuidField
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if idx, ok := wanted[entry.UUID]; ok {
			result := make([]byte, len(line))
			copy(result, line)
			results[idx] = result
			found++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning transcript: %w", err)
	}

	if found != len(uuids) {
		var missing []string
		for _, u := range uuids {
			if results[wanted[u]] == nil {
				missing = append(missing, u)
			}
		}
		return results, fmt.Errorf("missing %d UUIDs: %v", len(missing), missing)
	}

	return results, nil
}

// GetAgentTranscript returns the full contents of agent-{shortId}.jsonl.
func GetAgentTranscript(sessionID, agentShortID string) ([]byte, error) {
	path := filepath.Join(store.RawDir(sessionID), "agent-"+agentShortID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading agent transcript: %w", err)
	}
	return data, nil
}

// GetSessionRange returns all JSONL lines between fromUUID and toUUID inclusive.
// If fromUUID is empty, starts from the beginning.
// If toUUID is empty, reads to the end.
func GetSessionRange(sessionID, fromUUID, toUUID string) ([][]byte, error) {
	path := transcriptPath(sessionID)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening transcript: %w", err)
	}
	defer f.Close()

	collecting := fromUUID == ""
	var results [][]byte

	scanner := newScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()

		if !collecting {
			var entry uuidField
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			if entry.UUID == fromUUID {
				collecting = true
				result := make([]byte, len(line))
				copy(result, line)
				results = append(results, result)
				if toUUID != "" && entry.UUID == toUUID {
					break
				}
				continue
			}
			continue
		}

		result := make([]byte, len(line))
		copy(result, line)
		results = append(results, result)

		if toUUID != "" {
			var entry uuidField
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			if entry.UUID == toUUID {
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning transcript: %w", err)
	}

	return results, nil
}

func transcriptPath(sessionID string) string {
	return filepath.Join(store.RawDir(sessionID), "transcript.jsonl")
}

func newScanner(f *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, maxScanBuffer)
	return scanner
}

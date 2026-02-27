package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/vladolaru/cabrero/internal/fitness"
)

var sourcesMu sync.Mutex

type sourcesFile struct {
	Sources []fitness.Source `json:"sources"`
}

func sourcesPath() string {
	return filepath.Join(Root(), "sources.json")
}

// ReadSources reads the sources file from disk.
// Returns an empty slice if the file doesn't exist.
func ReadSources() ([]fitness.Source, error) {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()

	return readSources()
}

func readSources() ([]fitness.Source, error) {
	data, err := os.ReadFile(sourcesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []fitness.Source{}, nil
		}
		return nil, err
	}

	var sf sourcesFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing sources.json: %w", err)
	}
	if sf.Sources == nil {
		sf.Sources = []fitness.Source{}
	}
	return sf.Sources, nil
}

// WriteSources writes the sources to disk atomically.
func WriteSources(sources []fitness.Source) error {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()

	return writeSources(sources)
}

func writeSources(sources []fitness.Source) error {
	sf := sourcesFile{Sources: sources}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(sourcesPath(), data, 0o644)
}

// UpdateSource applies fn to the source with the given name and writes back.
// Returns an error if no source with that name exists.
// Deprecated: Use UpdateSourceByIdentity for origin-aware updates.
func UpdateSource(name string, fn func(*fitness.Source)) error {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()

	sources, err := readSources()
	if err != nil {
		return err
	}

	for i := range sources {
		if sources[i].Name == name {
			fn(&sources[i])
			return writeSources(sources)
		}
	}

	return fmt.Errorf("source %q not found", name)
}

// UpdateSourceByIdentity applies fn to the source matching both name and origin,
// then writes back. This prevents cross-origin collisions when multiple sources
// share the same name under different origins.
func UpdateSourceByIdentity(name, origin string, fn func(*fitness.Source)) error {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()

	sources, err := readSources()
	if err != nil {
		return err
	}

	for i := range sources {
		if sources[i].Name == name && sources[i].Origin == origin {
			fn(&sources[i])
			return writeSources(sources)
		}
	}

	return fmt.Errorf("source %q (origin %q) not found", name, origin)
}

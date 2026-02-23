# Source Registry Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Auto-discover sources from classifier outputs at TUI startup, persist user classifications in `sources.json`, and wire source mutations to the persistence layer so the Sources tab in `cabrero review` actually shows data.

**Architecture:** Three layers — persistence (`store/sources.go`) following the `calibration.go` pattern, discovery by scanning `evaluations/*-classifier.json` for skill/CLAUDE.md signals, and TUI integration replacing the empty `sourceGroups` placeholder with real data. Origin is inferred from name patterns (colon-namespaced → plugin, bare → user, absolute paths → project/user).

**Tech Stack:** Go, `encoding/json`, `os`, `path/filepath`, `sync`

**Design doc:** `docs/plans/2026-02-23-source-registry-design.md`

---

### Task 1: Persistence layer — ReadSources / WriteSources

**Files:**
- Create: `internal/store/sources.go`
- Create: `internal/store/sources_test.go`

**Reference:** `internal/store/calibration.go` — follow the same pattern (package-level mutex, `sourcesFile` envelope, `AtomicWrite`).

**Step 1: Write the tests**

In `internal/store/sources_test.go`:

```go
package store

import (
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/fitness"
)

func TestReadSources_Empty(t *testing.T) {
	setupTestStore(t)

	sources, err := ReadSources()
	if err != nil {
		t.Fatalf("ReadSources: %v", err)
	}
	if sources == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(sources) != 0 {
		t.Errorf("got %d sources, want 0", len(sources))
	}
}

func TestWriteAndReadSources(t *testing.T) {
	setupTestStore(t)

	now := time.Now().UTC()
	sources := []fitness.Source{
		{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate", SessionCount: 5, ClassifiedAt: &now},
		{Name: "brainstorming", Origin: "plugin:superpowers", Ownership: "", Approach: "", SessionCount: 2},
	}

	if err := WriteSources(sources); err != nil {
		t.Fatalf("WriteSources: %v", err)
	}

	got, err := ReadSources()
	if err != nil {
		t.Fatalf("ReadSources: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sources, want 2", len(got))
	}
	if got[0].Name != "git-workflow" {
		t.Errorf("Name = %q, want %q", got[0].Name, "git-workflow")
	}
	if got[0].Ownership != "mine" {
		t.Errorf("Ownership = %q, want %q", got[0].Ownership, "mine")
	}
	if got[1].Name != "brainstorming" {
		t.Errorf("Name = %q, want %q", got[1].Name, "brainstorming")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -run TestReadSources_Empty -v`
Expected: FAIL — `ReadSources` undefined

**Step 3: Write the implementation**

In `internal/store/sources.go`:

```go
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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -run "TestReadSources_Empty|TestWriteAndReadSources" -v`
Expected: PASS

**Step 5: Commit**

```
feat(store): add sources.json persistence layer

ReadSources/WriteSources follow the calibration.go pattern:
package-level mutex, JSON envelope, atomic writes.
```

---

### Task 2: UpdateSource — read-modify-write with tests

**Files:**
- Modify: `internal/store/sources.go`
- Modify: `internal/store/sources_test.go`

**Step 1: Write the test**

Add to `internal/store/sources_test.go`:

```go
func TestUpdateSource(t *testing.T) {
	setupTestStore(t)

	sources := []fitness.Source{
		{Name: "my-skill", Origin: "user", Ownership: "", Approach: ""},
	}
	if err := WriteSources(sources); err != nil {
		t.Fatal(err)
	}

	err := UpdateSource("my-skill", func(s *fitness.Source) {
		s.Ownership = "mine"
		s.Approach = "iterate"
	})
	if err != nil {
		t.Fatalf("UpdateSource: %v", err)
	}

	got, err := ReadSources()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Ownership != "mine" {
		t.Errorf("Ownership = %q, want %q", got[0].Ownership, "mine")
	}
	if got[0].Approach != "iterate" {
		t.Errorf("Approach = %q, want %q", got[0].Approach, "iterate")
	}
}

func TestUpdateSource_NotFound(t *testing.T) {
	setupTestStore(t)

	err := UpdateSource("nonexistent", func(s *fitness.Source) {
		s.Ownership = "mine"
	})
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -run TestUpdateSource -v`
Expected: FAIL — `UpdateSource` undefined

**Step 3: Write the implementation**

Add to `internal/store/sources.go`:

```go
// UpdateSource applies fn to the source with the given name and writes back.
// Returns an error if no source with that name exists.
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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -run TestUpdateSource -v`
Expected: PASS

**Step 5: Commit**

```
feat(store): add UpdateSource read-modify-write helper
```

---

### Task 3: Origin inference — InferOrigin and InferOriginFromPath

These pure functions parse source names and file paths to determine the source name and origin string.

**Files:**
- Create: `internal/store/source_origin.go`
- Create: `internal/store/source_origin_test.go`

**Step 1: Write the tests**

Real data from classifier outputs shows these patterns:

Skill signal names:
- `brainstorming` → bare name
- `superpowers:brainstorming` → plugin:skill
- `pirategoat-tools:full-code-review` → plugin:skill
- `using-ghe` → bare name

CLAUDE.md paths:
- `/Users/vlad/.claude/CLAUDE.md` → user-level
- `/Users/vlad/Work/a8c/cabrero/CLAUDE.md` → project-level

Proposal target paths:
- `/Users/vlad/.claude/skills/using-ghe.md` → user skill
- `/Users/vlad/.claude/plugins/cache/superpowers-marketplace/superpowers/4.3.1/skills/writing-plans/SKILL.md` → plugin skill

In `internal/store/source_origin_test.go`:

```go
package store

import "testing"

func TestInferOrigin(t *testing.T) {
	tests := []struct {
		input      string
		wantName   string
		wantOrigin string
	}{
		// Colon-namespaced → plugin.
		{"superpowers:brainstorming", "brainstorming", "plugin:superpowers"},
		{"pirategoat-tools:full-code-review", "full-code-review", "plugin:pirategoat-tools"},

		// Bare name → user.
		{"brainstorming", "brainstorming", "user"},
		{"using-ghe", "using-ghe", "user"},
		{"write-like-a-pirategoat", "write-like-a-pirategoat", "user"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, origin := InferOrigin(tt.input)
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if origin != tt.wantOrigin {
				t.Errorf("origin = %q, want %q", origin, tt.wantOrigin)
			}
		})
	}
}

func TestInferOriginFromPath(t *testing.T) {
	home := "/Users/vlad"

	tests := []struct {
		path       string
		wantName   string
		wantOrigin string
	}{
		// User-level CLAUDE.md.
		{home + "/.claude/CLAUDE.md", "CLAUDE.md", "user"},

		// Project-level CLAUDE.md.
		{home + "/Work/a8c/cabrero/CLAUDE.md", "CLAUDE.md (cabrero)", "project:cabrero"},
		{home + "/Work/a8c/woo-payments/CLAUDE.md", "CLAUDE.md (woo-payments)", "project:woo-payments"},

		// User-level skill (flat file).
		{home + "/.claude/skills/using-ghe.md", "using-ghe", "user"},

		// User-level skill (directory with SKILL.md).
		{home + "/.claude/skills/git-workflow/SKILL.md", "git-workflow", "user"},

		// Plugin skill.
		{home + "/.claude/plugins/cache/superpowers-marketplace/superpowers/4.3.1/skills/writing-plans/SKILL.md", "writing-plans", "plugin:superpowers"},
		{home + "/.claude/plugins/cache/pirategoat-marketplace/pirategoat-tools/1.0.0/skills/code-review/SKILL.md", "code-review", "plugin:pirategoat-tools"},

		// Worktree project CLAUDE.md.
		{home + "/Work/a8c/cabrero/.worktrees/review-tui/CLAUDE.md", "CLAUDE.md (review-tui)", "project:review-tui"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			name, origin := InferOriginFromPath(tt.path, home)
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if origin != tt.wantOrigin {
				t.Errorf("origin = %q, want %q", origin, tt.wantOrigin)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -run "TestInferOrigin" -v`
Expected: FAIL — functions undefined

**Step 3: Write the implementation**

In `internal/store/source_origin.go`:

```go
package store

import (
	"path/filepath"
	"strings"
)

// InferOrigin parses a skill signal name (e.g., "superpowers:brainstorming")
// and returns (sourceName, origin).
//
// Colon-namespaced names → plugin origin; bare names → user origin.
func InferOrigin(signalName string) (name, origin string) {
	if i := strings.Index(signalName, ":"); i > 0 {
		return signalName[i+1:], "plugin:" + signalName[:i]
	}
	return signalName, "user"
}

// InferOriginFromPath parses an absolute file path and returns (sourceName, origin).
//
// Recognized patterns:
//   - ~/.claude/CLAUDE.md                                    → ("CLAUDE.md", "user")
//   - ~/.claude/skills/<name>.md                             → ("<name>", "user")
//   - ~/.claude/skills/<name>/SKILL.md                       → ("<name>", "user")
//   - ~/.claude/plugins/cache/<mkt>/<plugin>/<ver>/skills/<skill>/SKILL.md
//     → ("<skill>", "plugin:<plugin>")
//   - /any/path/to/<project>/CLAUDE.md                       → ("CLAUDE.md (<project>)", "project:<project>")
func InferOriginFromPath(path, homeDir string) (name, origin string) {
	claudeDir := filepath.Join(homeDir, ".claude")

	// Paths inside ~/.claude/
	if strings.HasPrefix(path, claudeDir+"/") {
		rel := path[len(claudeDir)+1:] // e.g., "CLAUDE.md", "skills/foo.md", "plugins/cache/..."

		// ~/.claude/CLAUDE.md
		if rel == "CLAUDE.md" {
			return "CLAUDE.md", "user"
		}

		// ~/.claude/plugins/cache/<mkt>/<plugin>/<ver>/skills/<skill>/SKILL.md
		if strings.HasPrefix(rel, "plugins/cache/") {
			parts := strings.Split(rel, "/")
			// parts: [plugins, cache, <mkt>, <plugin>, <ver>, skills, <skill>, SKILL.md]
			if len(parts) >= 8 && parts[5] == "skills" {
				plugin := parts[3]
				skill := parts[6]
				return skill, "plugin:" + plugin
			}
		}

		// ~/.claude/skills/<name>.md (flat file)
		if strings.HasPrefix(rel, "skills/") && !strings.Contains(rel[len("skills/"):], "/") {
			base := filepath.Base(rel)
			ext := filepath.Ext(base)
			return strings.TrimSuffix(base, ext), "user"
		}

		// ~/.claude/skills/<name>/SKILL.md (directory)
		if strings.HasPrefix(rel, "skills/") {
			parts := strings.Split(rel, "/")
			if len(parts) >= 3 {
				return parts[1], "user"
			}
		}
	}

	// Any other CLAUDE.md → project-level.
	base := filepath.Base(path)
	if base == "CLAUDE.md" {
		dir := filepath.Dir(path)
		project := filepath.Base(dir)
		return "CLAUDE.md (" + project + ")", "project:" + project
	}

	// Fallback: use the file's base name without extension.
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext), "user"
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -run "TestInferOrigin" -v`
Expected: PASS

**Step 5: Commit**

```
feat(store): add origin inference for source discovery

InferOrigin parses colon-namespaced skill names (plugin vs user).
InferOriginFromPath handles absolute paths for CLAUDE.md files,
user skills (~/.claude/skills/), and plugin skills.
```

---

### Task 4: Discovery — DiscoverSourcesFromEvaluations

Scans `evaluations/*-classifier.json` files, extracts skill/CLAUDE.md signals, and returns deduplicated sources with session counts.

**Files:**
- Create: `internal/store/source_discovery.go`
- Create: `internal/store/source_discovery_test.go`

**Step 1: Write the test**

The test creates fake classifier output files in a temp store, then runs discovery.

In `internal/store/source_discovery_test.go`:

```go
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vladolaru/cabrero/internal/pipeline"
)

func TestDiscoverSourcesFromEvaluations(t *testing.T) {
	setupTestStore(t)

	evalDir := filepath.Join(Root(), "evaluations")

	// Write two classifier outputs.
	c1 := pipeline.ClassifierOutput{
		SessionID: "sess-001",
		SkillSignals: []pipeline.ClassifierSkillSignal{
			{SkillName: "superpowers:brainstorming"},
			{SkillName: "using-ghe"},
		},
		ClaudeMdSignals: []pipeline.ClassifierClaudeMdSignal{
			{Path: "/tmp/test-home/.claude/CLAUDE.md"},
		},
	}
	c2 := pipeline.ClassifierOutput{
		SessionID: "sess-002",
		SkillSignals: []pipeline.ClassifierSkillSignal{
			{SkillName: "superpowers:brainstorming"}, // duplicate skill
		},
		ClaudeMdSignals: []pipeline.ClassifierClaudeMdSignal{
			{Path: "/tmp/test-home/Work/myproject/CLAUDE.md"},
			{Path: "/tmp/test-home/.claude/CLAUDE.md"}, // duplicate path
		},
	}

	writeClassifier(t, evalDir, "sess-001", c1)
	writeClassifier(t, evalDir, "sess-002", c2)

	sources, err := DiscoverSourcesFromEvaluations()
	if err != nil {
		t.Fatalf("DiscoverSourcesFromEvaluations: %v", err)
	}

	// Expect 4 unique sources: brainstorming (plugin:superpowers, 2 sessions),
	// using-ghe (user, 1), CLAUDE.md (user, 2), CLAUDE.md (myproject) (project:myproject, 1).
	if len(sources) != 4 {
		t.Fatalf("got %d sources, want 4; sources: %v", len(sources), sourceNames(sources))
	}

	byName := map[string]int{}
	for _, s := range sources {
		byName[s.Name] = s.SessionCount
	}

	if byName["brainstorming"] != 2 {
		t.Errorf("brainstorming sessions = %d, want 2", byName["brainstorming"])
	}
	if byName["using-ghe"] != 1 {
		t.Errorf("using-ghe sessions = %d, want 1", byName["using-ghe"])
	}
}

func writeClassifier(t *testing.T, dir, sessionID string, c pipeline.ClassifierOutput) {
	t.Helper()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sessionID+"-classifier.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sourceNames(sources []fitness.Source) []string {
	var names []string
	for _, s := range sources {
		names = append(names, s.Name)
	}
	return names
}
```

Note: Add the `fitness` import — `"github.com/vladolaru/cabrero/internal/fitness"`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestDiscoverSourcesFromEvaluations -v`
Expected: FAIL — function undefined

**Step 3: Write the implementation**

In `internal/store/source_discovery.go`:

```go
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/pipeline"
)

// sourceKey uniquely identifies a discovered source.
type sourceKey struct {
	Name   string
	Origin string
}

// DiscoverSourcesFromEvaluations scans classifier output files and extracts
// unique sources (skills and CLAUDE.md files) with session counts.
func DiscoverSourcesFromEvaluations() ([]fitness.Source, error) {
	evalDir := filepath.Join(Root(), "evaluations")
	entries, err := os.ReadDir(evalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []fitness.Source{}, nil
		}
		return nil, err
	}

	home, _ := os.UserHomeDir()

	// Track unique sources and their session counts.
	// Each classifier output represents one session's worth of signals.
	// A source seen multiple times within one classifier counts once per session.
	counts := map[sourceKey]int{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "-classifier.json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(evalDir, e.Name()))
		if err != nil {
			continue
		}

		var c pipeline.ClassifierOutput
		if err := json.Unmarshal(data, &c); err != nil {
			continue
		}

		// Deduplicate within this session.
		seen := map[sourceKey]bool{}

		for _, sig := range c.SkillSignals {
			name, origin := InferOrigin(sig.SkillName)
			k := sourceKey{Name: name, Origin: origin}
			seen[k] = true
		}

		for _, sig := range c.ClaudeMdSignals {
			name, origin := InferOriginFromPath(sig.Path, home)
			k := sourceKey{Name: name, Origin: origin}
			seen[k] = true
		}

		for k := range seen {
			counts[k]++
		}
	}

	sources := make([]fitness.Source, 0, len(counts))
	for k, count := range counts {
		sources = append(sources, fitness.Source{
			Name:         k.Name,
			Origin:       k.Origin,
			SessionCount: count,
			HealthScore:  -1, // unscored
		})
	}

	return sources, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestDiscoverSourcesFromEvaluations -v`
Expected: PASS

**Step 5: Commit**

```
feat(store): add source discovery from classifier outputs

Scans evaluations/*-classifier.json for skill and CLAUDE.md signals,
deduplicates per-session, and returns unique sources with session counts.
```

---

### Task 5: Merge logic — LoadAndMergeSources

Combines persisted sources (with classifications) and discovered sources (with session counts).

**Files:**
- Create: `internal/store/source_merge.go`
- Create: `internal/store/source_merge_test.go`

**Step 1: Write the test**

In `internal/store/source_merge_test.go`:

```go
package store

import (
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/fitness"
)

func TestMergeSources(t *testing.T) {
	now := time.Now().UTC()

	persisted := []fitness.Source{
		{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate", SessionCount: 3, ClassifiedAt: &now},
		{Name: "old-removed", Origin: "user", Ownership: "not_mine", Approach: "paused", SessionCount: 1, ClassifiedAt: &now},
	}

	discovered := []fitness.Source{
		{Name: "git-workflow", Origin: "user", SessionCount: 7},             // existing: update count, keep classification
		{Name: "brainstorming", Origin: "plugin:superpowers", SessionCount: 2}, // new: add unclassified
	}

	merged := MergeSources(persisted, discovered)

	if len(merged) != 3 {
		t.Fatalf("got %d sources, want 3", len(merged))
	}

	byName := map[string]fitness.Source{}
	for _, s := range merged {
		byName[s.Name] = s
	}

	// Existing source: classification preserved, session count updated.
	gw := byName["git-workflow"]
	if gw.Ownership != "mine" {
		t.Errorf("git-workflow ownership = %q, want %q", gw.Ownership, "mine")
	}
	if gw.SessionCount != 7 {
		t.Errorf("git-workflow sessions = %d, want 7", gw.SessionCount)
	}

	// Persisted-only source: retained.
	old := byName["old-removed"]
	if old.Ownership != "not_mine" {
		t.Errorf("old-removed ownership = %q, want %q", old.Ownership, "not_mine")
	}

	// New source: added unclassified.
	bs := byName["brainstorming"]
	if bs.Ownership != "" {
		t.Errorf("brainstorming ownership = %q, want empty", bs.Ownership)
	}
	if bs.SessionCount != 2 {
		t.Errorf("brainstorming sessions = %d, want 2", bs.SessionCount)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestMergeSources -v`
Expected: FAIL — function undefined

**Step 3: Write the implementation**

In `internal/store/source_merge.go`:

```go
package store

import (
	"github.com/vladolaru/cabrero/internal/fitness"
)

// MergeSources combines persisted sources (with user classifications) and
// discovered sources (with session counts). Classification is always preserved
// from persisted data; session counts are updated from discovery.
func MergeSources(persisted, discovered []fitness.Source) []fitness.Source {
	// Index persisted by name for O(1) lookup.
	byName := make(map[string]*fitness.Source, len(persisted))
	result := make([]fitness.Source, len(persisted))
	for i, s := range persisted {
		result[i] = s
		byName[s.Name] = &result[i]
	}

	// Merge discovered sources.
	for _, d := range discovered {
		if existing, ok := byName[d.Name]; ok {
			// Update session count and origin (discovery has fresher data).
			existing.SessionCount = d.SessionCount
			if existing.Origin == "" {
				existing.Origin = d.Origin
			}
		} else {
			// New source — add as unclassified.
			result = append(result, d)
			byName[d.Name] = &result[len(result)-1]
		}
	}

	return result
}

// LoadAndMergeSources reads persisted sources, runs discovery, merges,
// and writes the result back. Returns the merged sources.
func LoadAndMergeSources() ([]fitness.Source, error) {
	persisted, err := ReadSources()
	if err != nil {
		persisted = []fitness.Source{}
	}

	discovered, err := DiscoverSourcesFromEvaluations()
	if err != nil {
		discovered = []fitness.Source{}
	}

	merged := MergeSources(persisted, discovered)

	// Persist the merged result (non-fatal if this fails).
	_ = WriteSources(merged)

	return merged, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestMergeSources -v`
Expected: PASS

**Step 5: Commit**

```
feat(store): add source merge and LoadAndMergeSources

MergeSources preserves user classifications from persisted data while
updating session counts from discovery. New sources are added as
unclassified. LoadAndMergeSources orchestrates the full flow.
```

---

### Task 6: Grouping — GroupSources

Organizes a flat source slice into display groups.

**Files:**
- Modify: `internal/store/source_merge.go` (add GroupSources)
- Modify: `internal/store/source_merge_test.go` (add test)

**Step 1: Write the test**

Add to `internal/store/source_merge_test.go`:

```go
func TestGroupSources(t *testing.T) {
	now := time.Now().UTC()
	sources := []fitness.Source{
		{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate", ClassifiedAt: &now},
		{Name: "CLAUDE.md (cabrero)", Origin: "project:cabrero", Ownership: "mine", Approach: "iterate", ClassifiedAt: &now},
		{Name: "brainstorming", Origin: "plugin:superpowers", Ownership: "not_mine", Approach: "evaluate", ClassifiedAt: &now},
		{Name: "new-thing", Origin: "user", Ownership: "", Approach: ""},                        // unclassified
		{Name: "writing-plans", Origin: "plugin:superpowers", Ownership: "", Approach: ""},       // unclassified
	}

	groups := GroupSources(sources)

	// Expect: Unclassified (first), User-level, Project: cabrero, Plugin: superpowers
	if len(groups) < 2 {
		t.Fatalf("got %d groups, want at least 2", len(groups))
	}

	// First group should be unclassified (if any exist).
	if groups[0].Label != "Unclassified" {
		t.Errorf("first group label = %q, want %q", groups[0].Label, "Unclassified")
	}
	if len(groups[0].Sources) != 2 {
		t.Errorf("unclassified count = %d, want 2", len(groups[0].Sources))
	}

	// Verify classified sources are in their origin groups.
	found := map[string]bool{}
	for _, g := range groups[1:] {
		found[g.Label] = true
	}
	for _, want := range []string{"User-level", "Project: cabrero", "Plugin: superpowers"} {
		if !found[want] {
			t.Errorf("missing group %q", want)
		}
	}
}

func TestGroupSources_NoUnclassified(t *testing.T) {
	now := time.Now().UTC()
	sources := []fitness.Source{
		{Name: "git-workflow", Origin: "user", Ownership: "mine", Approach: "iterate", ClassifiedAt: &now},
	}

	groups := GroupSources(sources)

	// No unclassified group should appear.
	for _, g := range groups {
		if g.Label == "Unclassified" {
			t.Error("unexpected Unclassified group when no unclassified sources exist")
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestGroupSources -v`
Expected: FAIL — function undefined

**Step 3: Write the implementation**

Add to `internal/store/source_merge.go`:

```go
import (
	"sort"
	"strings"
)

// GroupSources organizes a flat source slice into display groups.
// Unclassified sources (ownership=="") are placed in a separate group first.
// Remaining sources are grouped by origin.
func GroupSources(sources []fitness.Source) []fitness.SourceGroup {
	var unclassified []fitness.Source
	byOrigin := map[string][]fitness.Source{}

	for _, s := range sources {
		if s.Ownership == "" {
			unclassified = append(unclassified, s)
		} else {
			byOrigin[s.Origin] = append(byOrigin[s.Origin], s)
		}
	}

	var groups []fitness.SourceGroup

	// Unclassified first (only if non-empty).
	if len(unclassified) > 0 {
		groups = append(groups, fitness.SourceGroup{
			Label:   "Unclassified",
			Origin:  "",
			Sources: unclassified,
		})
	}

	// Sort origin keys for stable output.
	origins := make([]string, 0, len(byOrigin))
	for o := range byOrigin {
		origins = append(origins, o)
	}
	sort.Strings(origins)

	for _, o := range origins {
		groups = append(groups, fitness.SourceGroup{
			Label:   originLabel(o),
			Origin:  o,
			Sources: byOrigin[o],
		})
	}

	return groups
}

// originLabel converts an origin string to a display label.
func originLabel(origin string) string {
	switch {
	case origin == "user":
		return "User-level"
	case strings.HasPrefix(origin, "project:"):
		return "Project: " + origin[len("project:"):]
	case strings.HasPrefix(origin, "plugin:"):
		return "Plugin: " + origin[len("plugin:"):]
	default:
		return origin
	}
}
```

Note: Update the import block at the top of `source_merge.go` to include `"sort"` and `"strings"`, and ensure `fitness.SourceGroup` is the type from `internal/fitness/source.go`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestGroupSources -v`
Expected: PASS

**Step 5: Commit**

```
feat(store): add GroupSources for display grouping

Organizes flat sources into SourceGroups: unclassified first,
then by origin (user, project, plugin) with human-readable labels.
```

---

### Task 7: TUI integration — wire sources at startup

**Files:**
- Modify: `internal/tui/tui.go:42-45`

**Step 1: Replace the empty placeholder**

In `internal/tui/tui.go`, replace lines 42-45:

```go
// Before:
// Future: reports := fitness.ListReports()
var reports []fitness.Report

var sourceGroups []fitness.SourceGroup
```

With:

```go
// Future: reports := fitness.ListReports()
var reports []fitness.Report

mergedSources, _ := store.LoadAndMergeSources()
sourceGroups := store.GroupSources(mergedSources)
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: clean compile (no errors)

**Step 3: Smoke test**

Run: `go run ./cmd/snapshot source-manager`
Expected: renders the source manager with any discovered sources (or empty if no classifier outputs exist in the test environment).

Note: The snapshot command uses test fixtures, not real data, so it will still show the fixture data. The real test is `cabrero review` with real data.

**Step 4: Commit**

```
feat(tui): populate source manager from discovered sources

Replace the empty sourceGroups placeholder with real data from
LoadAndMergeSources at TUI startup.
```

---

### Task 8: Persist mutations — ownership and approach changes

When the user changes ownership or toggles approach in the source manager, the change should persist to `sources.json`.

**Files:**
- Modify: `internal/tui/sources/update.go:243-277` (handleToggleFinished, handleOwnershipFinished)

**Step 1: Update handleToggleFinished**

In `internal/tui/sources/update.go`, modify `handleToggleFinished` to persist:

```go
func (m Model) handleToggleFinished(msg message.ToggleApproachFinished) (Model, tea.Cmd) {
	if msg.Err != nil {
		return m, func() tea.Msg {
			return message.StatusMessage{Text: "Toggle failed: " + msg.Err.Error()}
		}
	}
	// Update the source in our local state.
	for gi := range m.groups {
		for si := range m.groups[gi].Sources {
			if m.groups[gi].Sources[si].Name == msg.SourceName {
				m.groups[gi].Sources[si].Approach = msg.NewApproach
				// Persist to disk (non-fatal if it fails).
				_ = store.UpdateSource(msg.SourceName, func(s *fitness.Source) {
					s.Approach = msg.NewApproach
				})
				return m, nil
			}
		}
	}
	return m, nil
}
```

**Step 2: Update handleOwnershipFinished**

Similarly, modify `handleOwnershipFinished`:

```go
func (m Model) handleOwnershipFinished(msg message.SetOwnershipFinished) (Model, tea.Cmd) {
	if msg.Err != nil {
		return m, func() tea.Msg {
			return message.StatusMessage{Text: "Ownership change failed: " + msg.Err.Error()}
		}
	}
	for gi := range m.groups {
		for si := range m.groups[gi].Sources {
			if m.groups[gi].Sources[si].Name == msg.SourceName {
				m.groups[gi].Sources[si].Ownership = msg.NewOwnership
				// Persist to disk (non-fatal if it fails).
				now := time.Now().UTC()
				_ = store.UpdateSource(msg.SourceName, func(s *fitness.Source) {
					s.Ownership = msg.NewOwnership
					s.ClassifiedAt = &now
				})
				return m, nil
			}
		}
	}
	return m, nil
}
```

**Step 3: Add imports**

Add to the import block in `update.go`:

```go
"time"

"github.com/vladolaru/cabrero/internal/fitness"
"github.com/vladolaru/cabrero/internal/store"
```

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: clean compile

**Step 5: Commit**

```
feat(tui): persist source ownership and approach changes

Mutations in the source manager now write through to sources.json
via store.UpdateSource, so classifications survive across sessions.
```

---

### Task 9: Run all tests and verify

**Step 1: Run store tests**

Run: `go test ./internal/store/ -v`
Expected: all pass

**Step 2: Run TUI tests**

Run: `go test ./internal/tui/... -v`
Expected: all pass

**Step 3: Run snapshot tests**

Run: `go test ./cmd/snapshot/ -v`
Expected: all pass

**Step 4: Build and smoke test**

Run: `go build ./... && make install`

Then: `cabrero review` — navigate to Sources tab, verify sources appear.

**Step 5: Commit (if any fixups needed)**

---

### Task 10: Update CHANGELOG.md

**Files:**
- Modify: `CHANGELOG.md`

**Step 1: Add entry under [Unreleased]**

Add to the `### Added` section (or create one):

```markdown
### Added
- Source registry: auto-discover skills and CLAUDE.md files from classifier outputs, persist ownership/approach classifications across sessions
```

**Step 2: Commit**

```
docs: add source registry to changelog
```

---

## Verification Checklist

1. `go build ./...` — compiles clean
2. `go test ./internal/store/ -v` — all source persistence/discovery/merge/grouping tests pass
3. `go test ./internal/tui/... -v` — existing TUI tests still pass
4. `go test ./cmd/snapshot/ -v` — snapshot height invariant tests still pass
5. `cabrero review` → Sources tab shows discovered sources
6. Toggle approach / set ownership → restart `cabrero review` → changes persist

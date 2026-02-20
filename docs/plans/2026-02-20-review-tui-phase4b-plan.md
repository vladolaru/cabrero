# Phase 4b: Review TUI (Assessment & Management) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Fitness Report Detail view and Source Manager view to the Review TUI — enabling assessment of third-party artifact health and centralized source lifecycle management with rollback.

**Architecture:** Extends the Phase 4a TUI with two new view packages (`fitness/`, `sources/`) under `internal/tui/`. Each follows the same Bubble Tea child model pattern: `model.go` + `update.go` + `view.go`. Data types for fitness reports and sources are new domain packages. All data injected via constructors — no filesystem reads in models.

**Tech Stack:** Same as Phase 4a — Bubble Tea v1.x, Bubbles v1.x, Lip Gloss v1.x. New: `charmbracelet/bubbles/progress` for fitness bars.

**Design Document:** `docs/plans/2026-02-20-review-tui-design.md` — Sections 4 (Fitness Report Detail) and 5 (Source Manager) are the source of truth.

---

## Task 1: Add Domain Types — FitnessReport and Source

**Files:**
- Create: `internal/fitness/fitness.go`
- Create: `internal/fitness/source.go`

### fitness.go

These types represent EVALUATE-mode pipeline output for third-party artifacts. No fitness reports exist in the current pipeline — these are new domain types that the pipeline will produce in the future. For now, we define the types the TUI needs to render.

```go
package fitness

import "time"

// Report is a fitness assessment for a third-party artifact (EVALUATE mode output).
type Report struct {
    ID              string        `json:"id"`
    SourceName      string        `json:"sourceName"`
    SourceOrigin    string        `json:"sourceOrigin"`    // "user", "project:<name>", "plugin:<name>"
    Ownership       string        `json:"ownership"`       // "mine" or "not_mine"
    ObservedCount   int           `json:"observedCount"`   // number of sessions observed
    WindowDays      int           `json:"windowDays"`      // observation window
    Assessment      Assessment    `json:"assessment"`
    Verdict         string        `json:"verdict"`         // plain-language summary
    Evidence        []EvidenceGroup `json:"evidence"`
    GeneratedAt     time.Time     `json:"generatedAt"`
}

// Assessment holds the three-bucket health breakdown.
type Assessment struct {
    Followed   BucketStat `json:"followed"`    // used correctly
    WorkedAround BucketStat `json:"workedAround"` // manually overridden
    Confused   BucketStat `json:"confused"`    // caused errors/retries
}

// BucketStat holds count and percentage for one bucket.
type BucketStat struct {
    Count   int     `json:"count"`
    Percent float64 `json:"percent"` // 0-100
}

// EvidenceGroup groups session evidence by category.
type EvidenceGroup struct {
    Category string          `json:"category"` // "followed", "worked_around", "confused"
    Entries  []EvidenceEntry `json:"entries"`
    Expanded bool            `json:"-"` // UI state, not persisted
}

// EvidenceEntry is one session's evidence.
type EvidenceEntry struct {
    SessionID   string    `json:"sessionId"`
    Timestamp   time.Time `json:"timestamp"`
    Summary     string    `json:"summary"`
    Detail      string    `json:"detail"`
}
```

### source.go

Source classification registry. Sources are artifacts Cabrero tracks.

```go
package fitness

import "time"

// Source represents a tracked artifact in the source registry.
type Source struct {
    Name         string    `json:"name"`
    Origin       string    `json:"origin"`       // "user", "project:<name>", "plugin:<name>"
    Ownership    string    `json:"ownership"`     // "mine", "not_mine", ""(unclassified)
    Approach     string    `json:"approach"`      // "iterate", "evaluate", "paused"
    SessionCount int       `json:"sessionCount"`
    HealthScore  float64   `json:"healthScore"`   // 0-100, -1 for unclassified
    ClassifiedAt *time.Time `json:"classifiedAt,omitempty"`
}

// SourceGroup groups sources by origin for display.
type SourceGroup struct {
    Label     string   `json:"label"`     // display label ("User-level", "Project: foo")
    Origin    string   `json:"origin"`    // raw origin key
    Sources   []Source `json:"sources"`
    Collapsed bool     `json:"-"`         // UI state
}

// ChangeEntry records a historical change for rollback tracking.
type ChangeEntry struct {
    ID           string    `json:"id"`
    SourceName   string    `json:"sourceName"`
    ProposalID   string    `json:"proposalId"`
    Description  string    `json:"description"`
    Timestamp    time.Time `json:"timestamp"`
    Status       string    `json:"status"`      // "approved", "rejected"
    PreviousContent string `json:"previousContent,omitempty"` // for rollback
    FilePath     string    `json:"filePath"`
}

// ListSources returns all known sources. For now, returns empty slice.
// Future: reads from ~/.cabrero/sources.json.
func ListSources() ([]Source, error) {
    return nil, nil
}

// ListSourceGroups returns sources organized into groups by origin.
func ListSourceGroups(sources []Source) []SourceGroup {
    // Group by origin, unclassified at bottom
}

// ListChanges returns change history for a source. For now, returns empty slice.
func ListChanges(sourceName string) ([]ChangeEntry, error) {
    return nil, nil
}
```

**Commit:** `feat(fitness): add domain types for fitness reports and source registry`

---

## Task 2: Extend Message Types for Phase 4b

**Files:**
- Modify: `internal/tui/message/message.go`

Add new ViewState values and messages for fitness and source views.

**New ViewState values:**
```go
const (
    ViewDashboard ViewState = iota
    ViewProposalDetail
    ViewFitnessDetail    // NEW
    ViewSourceManager    // NEW
    ViewSourceDetail     // NEW
)
```

**New messages:**
```go
// Fitness report actions
type DismissFinished struct{ ReportID string; Err error }
type JumpToSources struct{ SourceName string } // pre-select source in manager

// Source manager actions
type ToggleApproachFinished struct{ SourceName string; NewApproach string; Err error }
type SetOwnershipFinished struct{ SourceName string; NewOwnership string; Err error }
type ClassifyFinished struct{ SourceName string; Err error }
type RollbackFinished struct{ ChangeID string; Err error }
```

**Commit:** `feat(tui): add Phase 4b message types for fitness and sources`

---

## Task 3: Extend Config and KeyMap for Phase 4b

**Files:**
- Modify: `internal/tui/shared/config.go`
- Modify: `internal/tui/shared/keys.go`
- Modify: `internal/tui/shared/config_test.go`

### Config additions

Add `SourceManager` config section:
```go
type SourceManagerConfig struct {
    GroupCollapsedDefault bool `json:"groupCollapsedDefault"`
}
```

Add to `Config` struct:
```go
SourceManager SourceManagerConfig `json:"sourceManager"`
```

Update `DefaultConfig()` to include:
```go
SourceManager: SourceManagerConfig{
    GroupCollapsedDefault: false,
},
```

### KeyMap additions

Add to `KeyMap` struct:
```go
// Fitness Report
Dismiss key.Binding

// Source Manager
ToggleApproach key.Binding
SetOwnership   key.Binding
Rollback       key.Binding
CollapseGroup  key.Binding
ExpandGroup    key.Binding
```

Add to `NewKeyMap()`:
```go
Dismiss:        key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "dismiss")),
ToggleApproach: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle mode")),
SetOwnership:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "set ownership")),
Rollback:       key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "rollback")),
CollapseGroup:  // shares Left binding
ExpandGroup:    // shares Right binding
```

Add `FitnessShortHelp()` and `SourcesShortHelp()` methods.

**Tests:** Verify defaults include new config fields. Verify new key bindings exist.

**Commit:** `feat(tui): extend config and key bindings for Phase 4b views`

---

## Task 4: Assessment Bar Component

**Files:**
- Create: `internal/tui/components/assessbar.go`
- Create: `internal/tui/components/assessbar_test.go`

Pure rendering function for fitness assessment bars.

```go
// RenderAssessBar renders a horizontal fitness bar.
// percent is 0-100, width is the total character width available.
// color determines the filled portion color (green/yellow/red based on category).
func RenderAssessBar(percent float64, width int, category string) string

// RenderAssessment renders the full three-row assessment with labels and bars.
func RenderAssessment(assessment fitness.Assessment, width int) string
```

Layout per row:
```
  Followed correctly ..... 5 sessions  ████████░░░░░░░░░░░░░░░░  36%
```

Color mapping:
- "followed" → green (`shared.ColorSuccess`)
- "worked_around" → warning/amber (`shared.ColorWarning`)
- "confused" → red (`shared.ColorError`)

Bar characters: `█` for filled, `░` for empty.

**Tests:**
- `TestRenderAssessBar_Full` — 100% renders all filled
- `TestRenderAssessBar_Empty` — 0% renders all empty
- `TestRenderAssessBar_Half` — 50% renders half and half
- `TestRenderAssessment_ThreeBuckets` — renders all three rows correctly

**Commit:** `feat(tui): add assessment bar component for fitness reports`

---

## Task 5: Fitness Report Detail Model

**Files:**
- Create: `internal/tui/fitness/model.go`
- Create: `internal/tui/fitness/update.go`
- Create: `internal/tui/fitness/view.go`
- Create: `internal/tui/fitness/testmain_test.go`
- Create: `internal/tui/fitness/model_test.go`

### model.go

```go
type Model struct {
    report       *fitness.Report
    viewport     viewport.Model
    evidence     []fitness.EvidenceGroup
    focus        Focus
    spinner      spinner.Model
    width        int
    height       int
    keys         *shared.KeyMap
    config       *shared.Config
}

type Focus int
const (
    FocusReport Focus = iota
    FocusChat
)

func New(report *fitness.Report, keys *shared.KeyMap, cfg *shared.Config) Model
func (m *Model) SetSize(width, height int)
```

### update.go

Key handling:
- `x` — Dismiss (archive report)
- `s` — Jump to source manager (`JumpToSources` message)
- `c` — Focus chat
- `Tab` — Toggle focus between report and chat
- `Enter` — Toggle evidence group expanded
- Up/Down — Scroll viewport

### view.go

Layout from design doc Section 4:
- Header: source name, ownership, mode, observed count
- ASSESSMENT section: three assessment bars via `RenderAssessment()`
- VERDICT section: plain-language text
- SESSION EVIDENCE section: grouped by category, expandable entries
- Status bar with context-sensitive keys

### testmain_test.go

```go
func TestMain(m *testing.M) {
    os.Setenv("NO_COLOR", "1")
    os.Exit(m.Run())
}
```

### Tests:
- `TestFitness_EvidenceExpand` — Enter toggles evidence group expanded
- `TestFitness_DismissEmitsMessage` — `x` sends DismissFinished
- `TestFitness_JumpToSources` — `s` sends JumpToSources message

**Commit:** `feat(tui): add fitness report detail model`

---

## Task 6: Source Manager Model

**Files:**
- Create: `internal/tui/sources/model.go`
- Create: `internal/tui/sources/update.go`
- Create: `internal/tui/sources/view.go`
- Create: `internal/tui/sources/testmain_test.go`
- Create: `internal/tui/sources/model_test.go`

### model.go

```go
type Model struct {
    groups       []fitness.SourceGroup
    flatItems    []flatItem  // flattened for cursor navigation
    cursor       int
    detailOpen   bool
    detailSource *fitness.Source
    changes      []fitness.ChangeEntry
    confirm      components.ConfirmModel
    confirmState ConfirmState
    width        int
    height       int
    keys         *shared.KeyMap
    config       *shared.Config
}

// flatItem maps a visible row to either a group header or source entry.
type flatItem struct {
    isHeader bool
    groupIdx int
    sourceIdx int  // -1 for headers
}

type ConfirmState int
const (
    ConfirmNone ConfirmState = iota
    ConfirmToggleApproach
    ConfirmSetOwnership
    ConfirmRollback
)

func New(groups []fitness.SourceGroup, keys *shared.KeyMap, cfg *shared.Config) Model
func (m *Model) SetSize(width, height int)
func (m Model) SelectedSource() *fitness.Source
func (m Model) PreSelectSource(name string) Model
```

### update.go

Key handling:
- Up/Down — Navigate flat list (skip group headers or land on them)
- `t` — Toggle approach (iterate/evaluate) with confirm
- `o` — Set ownership prompt ([m]ine / [n]ot mine)
- `Enter` on source — Open detail sub-view (shows changes, rollback)
- `Enter` on unclassified — Classification prompt
- Left/Right — Collapse/expand group
- `z` — Rollback (in detail view, requires `rollbackRequiresConfirm`)
- Esc — Close detail / go back

### view.go

Layout from design doc Section 5:
- Header: source counts (total, iterate, evaluate, unclassified)
- Table with columns: SOURCE, OWNERSHIP, APPROACH, SESSIONS, HEALTH
- Groups with collapsible headers
- Unclassified at bottom with `⚠`
- Detail sub-view when open: recent changes list with rollback option

Health bar rendering:
- Iterate sources: approval ratio bar (green filled)
- Evaluate sources: fitness percentage bar (colored by score)
- Unclassified: `───`

Adaptive columns:
- Wide (>=120): all 5 columns
- Standard (80-119): 4 columns (no health)
- Narrow (<80): 3 columns (name, mode, sessions)

### Tests:
- `TestSources_Navigation` — cursor moves through flat items correctly
- `TestSources_GroupCollapse` — Left collapses group, Right expands
- `TestSources_ToggleApproach` — `t` triggers confirm, then emits ToggleApproachFinished
- `TestSources_PreSelect` — PreSelectSource positions cursor correctly
- `TestSources_EmptyState` — Shows "Every goat has a name." when all classified

**Commit:** `feat(tui): add source manager model with groups and rollback`

---

## Task 7: Test Data Fixtures for Phase 4b

**Files:**
- Modify: `internal/tui/testdata/fixtures.go`

Add factory functions for fitness reports and sources:

```go
func TestFitnessReport(overrides ...func(*fitness.Report)) *fitness.Report
func TestSource(overrides ...func(*fitness.Source)) fitness.Source
func TestSourceGroups() []fitness.SourceGroup
func TestChangeEntries() []fitness.ChangeEntry
```

Realistic test data matching design doc examples:
- Fitness report for "some-third-party/docx-helper" with 14 sessions, low fitness
- Source groups: User-level (3), Project: woo-payments (2), Plugin: some-third-party (2), Unclassified (2)
- Change history: 2 approved, 1 rejected entries

**Commit:** `test(tui): add Phase 4b test data fixtures`

---

## Task 8: Integrate New Views into Root Model

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/tui.go`

### model.go changes

Add new child models:
```go
type reviewModel struct {
    // ... existing fields ...
    fitness  fitness_tui.Model  // new
    sources  sources_tui.Model  // new
}
```

Update `pushView()` to handle new ViewState values:
- `ViewFitnessDetail`: initialize fitness model from selected item
- `ViewSourceManager`: initialize source manager model, optionally pre-select source
- `ViewSourceDetail`: (handled within sources model)

Handle new messages in `Update()`:
- `DismissFinished` — archive report, return to dashboard with status
- `JumpToSources` — push ViewSourceManager with pre-selection
- `ToggleApproachFinished` — update source in model
- `SetOwnershipFinished` — update source in model
- `RollbackFinished` — status message, refresh changes list

### tui.go changes

Load fitness reports and sources at startup (for now: empty lists since these data types don't exist in the pipeline yet):
```go
fitnessReports := []fitness.Report{} // future: fitness.ListReports()
sourceGroups := fitness.ListSourceGroups(sources)
```

### Dashboard integration

Add `s` key handler in dashboard to push ViewSourceManager.

Handle fitness report items in the dashboard:
- Fitness reports appear with `◎` indicator
- Enter on fitness report pushes ViewFitnessDetail

**Commit:** `feat(tui): integrate fitness and source views into root model`

---

## Task 9: Dashboard Updates for Phase 4b

**Files:**
- Modify: `internal/tui/dashboard/model.go`
- Modify: `internal/tui/dashboard/update.go`
- Modify: `internal/tui/dashboard/view.go`

### Mixed item list

The dashboard now shows both proposals and fitness reports in a unified list. Add a wrapper type:

```go
type DashboardItem struct {
    Proposal       *pipeline.ProposalWithSession
    FitnessReport  *fitness.Report
}

func (d DashboardItem) IsProposal() bool
func (d DashboardItem) IsFitnessReport() bool
func (d DashboardItem) TypeIndicator() string  // "●" or "◎"
func (d DashboardItem) TypeName() string
func (d DashboardItem) Target() string
func (d DashboardItem) Age() string
```

Update `New()` to accept fitness reports alongside proposals:
```go
func New(proposals []pipeline.ProposalWithSession, reports []fitness.Report,
    stats message.DashboardStats, keys *shared.KeyMap, cfg *shared.Config) Model
```

### update.go

Add `s` key to open source manager:
```go
case key.Matches(msg, m.keys.Sources):
    return m, func() tea.Msg {
        return message.PushView{View: message.ViewSourceManager}
    }
```

Enter on a fitness report item should push ViewFitnessDetail instead of ViewProposalDetail.

### view.go

Update the proposal list renderer to handle mixed items:
- Proposals: `●` indicator in accent color
- Fitness reports: `◎` indicator in warning color
- Different actions shown for fitness reports in status bar

Update stats header to include fitness report count.

**Commit:** `feat(tui): update dashboard for mixed proposals and fitness reports`

---

## Task 10: Flavor Text Extensions

**Files:**
- Modify: `internal/tui/components/flavor.go`

Add new flavor text for Phase 4b states:

```go
// EmptyFitnessEvidence returns the message when no evidence entries exist.
func EmptyFitnessEvidence() string

// EmptySources returns the message when no sources are tracked.
func EmptySources() string

// AllClassified returns the message when all sources are classified.
func AllClassified() string

// ConfirmDismiss returns the status bar message after dismissing a fitness report.
func ConfirmDismiss() string

// ConfirmRollback returns the status bar message after rolling back a change.
func ConfirmRollback() string
```

With flavor enabled:
- EmptyFitnessEvidence: "No sessions witnessed. The goat was never observed."
- EmptySources: "No artifacts tracked yet. The flock is empty."
- AllClassified: "Every goat has a name."
- ConfirmDismiss: "Report acknowledged. The goatherd moves on."
- ConfirmRollback: "Change undone. The flock forgets."

Without flavor:
- Neutral fallbacks like "No evidence entries.", "No sources tracked.", etc.

**Commit:** `feat(tui): add Phase 4b flavor text`

---

## Task 11: Integration Tests for Phase 4b

**Files:**
- Modify: `internal/tui/integration_test.go`

New tests:

```go
func TestDashboardToFitnessAndBack(t *testing.T) {
    // Navigate to fitness report item, enter, verify ViewFitnessDetail, esc back
}

func TestDashboardToSourceManager(t *testing.T) {
    // Press 's', verify ViewSourceManager, esc back to dashboard
}

func TestFitnessJumpToSources(t *testing.T) {
    // In fitness detail, press 's', verify jump to source manager with pre-selection
}

func TestSourceManagerGroupCollapse(t *testing.T) {
    // Navigate to group, collapse with left, verify fewer visible items
}
```

**Commit:** `test(tui): add Phase 4b integration tests`

---

## Task 12: Wire Everything & Update Docs

**Files:**
- Modify: `DESIGN.md` — Mark Phase 4b as implemented
- Modify: `CHANGELOG.md` — Add Phase 4b entries under [Unreleased]

### CHANGELOG entries:
- Fitness Report Detail view with assessment bars and session evidence
- Source Manager with grouping, ownership/approach toggles, and rollback
- Dashboard mixed item list supporting both proposals and fitness reports
- `s` keyboard shortcut to open Source Manager from dashboard

### DESIGN.md updates:
- Phase 4b status → implemented

**Commit:** `docs: update DESIGN.md and CHANGELOG.md for Phase 4b`

---

## Verification

After all tasks are complete:

1. **Build:** `go build ./...` — must succeed with zero errors
2. **Tests:** `go test ./...` — all tests must pass
3. **Manual smoke test:**
   - `cabrero review` — dashboard should show (empty fitness report section is fine)
   - Press `s` — should navigate to Source Manager (empty state)
   - Press `Esc` — back to dashboard
   - Press `?` — help overlay should show new keybindings
4. **Existing tests still pass:** All Phase 4a tests must remain green

---

## Task Order & Dependencies

```
Task 1 (domain types) → Task 2 (messages) → Task 3 (config + keys)
                                                    ↓
Task 4 (assess bar) → Task 5 (fitness model) ─────→ Task 7 (test fixtures)
                       Task 6 (sources model) ──┘
                                                    ↓
Task 9 (dashboard updates) ← Task 8 (root model integration)
                                                    ↓
Task 10 (flavor text) → Task 11 (integration tests) → Task 12 (docs)
```

Tasks 1-3 are foundation. Tasks 4-6 are the two new views plus the bar component. Task 7 provides test data. Task 8 wires them into the root. Tasks 9-12 are the final layer.

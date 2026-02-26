# Implement Dashboard Filter — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the dashboard filter actually filter — `applyFilter()` currently ignores the filter text entirely. Also fix the hardcoded escape/enter keys in `updateFilter()` to use the shared keymap.

**Architecture:** The filter input already exists (`textinput.Model`, `filterActive bool`, `filterText string`). The gap is in `applyFilter()`, which sets `m.filtered = m.items` regardless of `m.filterText`. The fix adds a `matchesFilter(item DashboardItem, text string) bool` function and calls it in `applyFilter()`. Syntax: `type:<val>` matches `TypeName()`, `target:<val>` matches `Target()`, free text matches any field (type, target, confidence). All matching is case-insensitive.

**Dependency note:** If `bubbles/list` adoption is being considered (see architecture analysis), decide on that *before* implementing this plan. Adopting `bubbles/list` gives filter-as-you-type for free and makes this plan redundant.

**Tech Stack:** Go, existing `DashboardItem` methods, no new dependencies.

---

### Task 1: Fix hardcoded keys in `updateFilter()`

**Files:**
- Modify: `internal/tui/dashboard/update.go`
- Test: `internal/tui/dashboard/model_test.go`

**Step 1: Write a failing test**

Add to `internal/tui/dashboard/model_test.go`:

```go
func TestDashboard_Filter_EscapeUsesKeymap(t *testing.T) {
	// Verify that the keymap's Back binding (not a hardcoded "esc") closes the filter.
	// Test with a vim keymap where Back is still "esc" — this ensures the code path
	// goes through key.Matches rather than a raw string comparison.
	keys := shared.NewKeyMap("vim")
	cfg := testdata.TestConfig()
	m := New(testdata.TestProposals(), nil, testdata.TestDashboardStats(), &keys, cfg)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Open filter.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.filterActive {
		t.Fatal("filter should be active after '/'")
	}

	// Dismiss with Escape.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.filterActive {
		t.Error("filter should be dismissed after Esc")
	}
}
```

**Step 2: Verify it passes already** (this test may pass with current code since esc works). The real test is below.

**Step 3: Harden the implementation**

In `internal/tui/dashboard/update.go`, `updateFilter()`, replace hardcoded bindings:

```go
func (m Model) updateFilter(msg tea.Msg) (Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(msg, m.keys.Back): // was: key.NewBinding(key.WithKeys("esc"))
			m.filterActive = false
			m.filterInput.Blur()
			m.filterText = ""
			m.filterInput.SetValue("")
			m.applyFilter()
			return m, nil
		case key.Matches(msg, m.keys.Open): // was: key.NewBinding(key.WithKeys("enter"))
			m.filterActive = false
			m.filterInput.Blur()
			m.filterText = m.filterInput.Value()
			m.applyFilter()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	return m, cmd
}
```

**Step 4: Verify tests still pass**

```bash
cd internal/tui/dashboard && go test ./...
```

Expected: all pass.

**Step 5: Commit**

```bash
git add internal/tui/dashboard/update.go
git commit -m "fix(dashboard): use keymap bindings in filter instead of hardcoded keys"
```

---

### Task 2: Implement `matchesFilter()`

**Files:**
- Modify: `internal/tui/dashboard/model.go`
- Test: `internal/tui/dashboard/model_test.go`

**Step 1: Write failing tests for filter logic**

Add to `internal/tui/dashboard/model_test.go`:

```go
func TestDashboard_Filter_FreeText(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// All items visible before filtering.
	initialCount := len(m.filtered)
	if initialCount == 0 {
		t.Fatal("need items to test filter")
	}

	// Filter by a type name that only some items have.
	m.filterText = "skill_improvement"
	m.applyFilter()

	for _, item := range m.filtered {
		fields := strings.ToLower(item.TypeName() + " " + item.Target() + " " + item.Confidence())
		if !strings.Contains(fields, "skill_improvement") {
			t.Errorf("filtered item %q doesn't match filter %q", item.TypeName(), "skill_improvement")
		}
	}
}

func TestDashboard_Filter_TypePrefix(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m.filterText = "type:skill"
	m.applyFilter()

	for _, item := range m.filtered {
		if !strings.Contains(strings.ToLower(item.TypeName()), "skill") {
			t.Errorf("item type %q doesn't match type:skill filter", item.TypeName())
		}
	}
}

func TestDashboard_Filter_TargetPrefix(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m.filterText = "target:docx"
	m.applyFilter()

	for _, item := range m.filtered {
		if !strings.Contains(strings.ToLower(item.Target()), "docx") {
			t.Errorf("item target %q doesn't match target:docx filter", item.Target())
		}
	}
}

func TestDashboard_Filter_EmptyText_ShowsAll(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	totalBefore := len(m.filtered)

	m.filterText = "nomatch_xyzzy_12345"
	m.applyFilter()
	if len(m.filtered) != 0 {
		t.Errorf("expected 0 results for no-match, got %d", len(m.filtered))
	}

	m.filterText = ""
	m.applyFilter()
	if len(m.filtered) != totalBefore {
		t.Errorf("clearing filter: got %d items, want %d", len(m.filtered), totalBefore)
	}
}

func TestDashboard_Filter_CaseInsensitive(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Lower case of a type name that exists.
	m.filterText = "SKILL_IMPROVEMENT"
	m.applyFilter()
	upper := len(m.filtered)

	m.filterText = "skill_improvement"
	m.applyFilter()
	lower := len(m.filtered)

	if upper != lower {
		t.Errorf("filter should be case-insensitive: UPPER=%d LOWER=%d", upper, lower)
	}
}
```

**Step 2: Verify tests fail**

```bash
cd internal/tui/dashboard && go test -run "TestDashboard_Filter" -v
```

Expected: `TestDashboard_Filter_FreeText`, `_TypePrefix`, `_TargetPrefix` FAIL because filter returns all items.

**Step 3: Add `matchesFilter()` to `dashboard/model.go`**

```go
// matchesFilter reports whether item matches the filter text.
// Supports "type:<val>" and "target:<val>" prefix tokens; otherwise
// matches against all searchable fields (type, target, confidence).
// Matching is case-insensitive.
func matchesFilter(item DashboardItem, text string) bool {
	if text == "" {
		return true
	}
	lower := strings.ToLower(text)

	if val, ok := strings.CutPrefix(lower, "type:"); ok {
		return strings.Contains(strings.ToLower(item.TypeName()), val)
	}
	if val, ok := strings.CutPrefix(lower, "target:"); ok {
		return strings.Contains(strings.ToLower(item.Target()), val)
	}

	// Free text: match against type, target, and confidence fields.
	haystack := strings.ToLower(item.TypeName() + " " + item.Target() + " " + item.Confidence())
	return strings.Contains(haystack, lower)
}
```

Note: `strings.CutPrefix` is available in Go 1.20+. Verify the project's Go version in `go.mod`. If below 1.20, use `strings.HasPrefix` + `[len("type:"):]` instead.

**Step 4: Update `applyFilter()` in `dashboard/model.go`**

```go
func (m *Model) applyFilter() {
	if m.filterText == "" {
		m.filtered = m.items
	} else {
		m.filtered = m.filtered[:0] // reuse the slice backing array
		for _, item := range m.items {
			if matchesFilter(item, m.filterText) {
				m.filtered = append(m.filtered, item)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.updateContent()
}
```

Add import `"strings"` to `model.go` if not already present.

**Step 5: Run filter tests**

```bash
cd internal/tui/dashboard && go test -run "TestDashboard_Filter" -v
```

Expected: all PASS.

**Step 6: Run all dashboard tests**

```bash
cd internal/tui/dashboard && go test ./...
```

Expected: all pass.

**Step 7: Commit**

```bash
git add internal/tui/dashboard/model.go internal/tui/dashboard/model_test.go
git commit -m "feat(dashboard): implement filter logic — type:/target: prefix and free text

applyFilter() previously ignored filterText entirely. Now:
- 'type:skill' matches items whose TypeName() contains 'skill'
- 'target:docx' matches items whose Target() contains 'docx'
- Free text matches across type, target, and confidence fields
- All matching is case-insensitive
- Empty filter restores the full list"
```

---

### Task 3: Wire filter text to real-time view feedback

The filter currently only applies when Enter is pressed. The filter input shows what the user typed but the list doesn't update until Enter. This is acceptable per the current design (the placeholder says "type:skill target:docx or free text..."), but the status bar should hint that the filter is active.

**Files:**
- Modify: `internal/tui/dashboard/view.go`
- Test: `internal/tui/dashboard/model_test.go`

**Step 1: Write test**

```go
func TestDashboard_Filter_ActiveViewShowsInput(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Open filter.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.filterActive {
		t.Fatal("filter should be active")
	}

	view := ansi.Strip(m.View())
	// The filter input prompt should appear.
	if !strings.Contains(view, "/ ") {
		t.Error("view should show filter prompt '/ ' when filter is active")
	}
}

func TestDashboard_Filter_ShowsFilteredCount(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Apply a narrow filter.
	m.filterText = "skill_improvement"
	m.applyFilter()

	filteredCount := len(m.filtered)
	totalCount := len(m.items)

	// The sort indicator should show filtered vs total when a filter is active.
	view := ansi.Strip(m.View())
	if filteredCount < totalCount {
		// Expect some indication of filtering.
		if !strings.Contains(view, "filtered") && !strings.Contains(view, fmt.Sprintf("%d", filteredCount)) {
			t.Logf("view = %s", view)
			t.Error("view should indicate filter is active (filtered count or 'filtered' label)")
		}
	}
}
```

**Step 2: Update `dashboard/view.go` — show filter indicator**

In `View()`, after the sort indicator and before the status/filter bar, add a filter active indicator:

```go
// In View(), where the sort line is rendered:
if len(m.filtered) > 0 {
    sortLine := fmt.Sprintf("  Sort: %s", m.sortOrder)
    if m.filterText != "" {
        sortLine += fmt.Sprintf("  ·  filter: %q (%d/%d)", m.filterText, len(m.filtered), len(m.items))
    }
    b.WriteString(mutedStyle.Render(sortLine))
    b.WriteString("\n")
}
```

**Step 3: Run all tests**

```bash
cd internal/tui/dashboard && go test ./...
```

Expected: all pass. Update the `TestDashboard_View80x24` test if it asserts on exact sort line content.

**Step 4: Commit**

```bash
git add internal/tui/dashboard/view.go internal/tui/dashboard/model_test.go
git commit -m "feat(dashboard): show active filter in sort indicator line"
```

---

### Task 4: Full smoke test

```bash
make install && cabrero
```

1. Open the dashboard. Press `/`.
2. Type `type:skill` — press Enter — confirm only skill proposals remain.
3. Press `/`, type `target:docx`, press Enter — confirm only docx-related items show.
4. Press `/`, clear with Backspace, press Enter — confirm all items return.
5. Press `/`, type `xyzzy_no_match`, press Enter — confirm empty state shows.
6. Press `/`, type something, press Esc — confirm filter is cleared and all items return.
7. Confirm the sort indicator shows the filter summary when active.

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/store"
)

func TestSources_List_Empty(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := sourcesRun([]string{"list"}, &buf)
	if err != nil {
		t.Fatalf("sources list: %v", err)
	}
	if !strings.Contains(buf.String(), "No sources") {
		t.Error("expected empty sources message")
	}
}

func TestSources_List_ShowsSources(t *testing.T) {
	setupConfigTest(t)

	if err := store.WriteSources([]fitness.Source{
		{Name: "CLAUDE.md", Origin: "user", Ownership: "mine", Approach: "iterate"},
		{Name: "debug-skill", Origin: "user", Ownership: "not_mine", Approach: "evaluate"},
	}); err != nil {
		t.Fatalf("writing sources: %v", err)
	}

	var buf bytes.Buffer
	err := sourcesRun([]string{"list"}, &buf)
	if err != nil {
		t.Fatalf("sources list: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "CLAUDE.md") {
		t.Error("expected CLAUDE.md in output")
	}
	if !strings.Contains(out, "debug-skill") {
		t.Error("expected debug-skill in output")
	}
}

func TestSources_SetOwnership(t *testing.T) {
	setupConfigTest(t)

	if err := store.WriteSources([]fitness.Source{
		{Name: "test-skill", Origin: "user", Ownership: "", Approach: ""},
	}); err != nil {
		t.Fatalf("writing sources: %v", err)
	}

	var buf bytes.Buffer
	err := sourcesRun([]string{"set-ownership", "test-skill", "mine"}, &buf)
	if err != nil {
		t.Fatalf("set-ownership: %v", err)
	}

	sources, _ := store.ReadSources()
	if sources[0].Ownership != "mine" {
		t.Errorf("ownership = %q, want mine", sources[0].Ownership)
	}
}

func TestSources_SetApproach(t *testing.T) {
	setupConfigTest(t)

	if err := store.WriteSources([]fitness.Source{
		{Name: "test-skill", Origin: "user", Ownership: "mine", Approach: "iterate"},
	}); err != nil {
		t.Fatalf("writing sources: %v", err)
	}

	var buf bytes.Buffer
	err := sourcesRun([]string{"set-approach", "test-skill", "paused"}, &buf)
	if err != nil {
		t.Fatalf("set-approach: %v", err)
	}

	sources, _ := store.ReadSources()
	if sources[0].Approach != "paused" {
		t.Errorf("approach = %q, want paused", sources[0].Approach)
	}
}

func TestSources_SetOwnership_InvalidValue(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := sourcesRun([]string{"set-ownership", "test", "invalid"}, &buf)
	if err == nil {
		t.Error("expected error for invalid ownership value")
	}
}

func TestSources_SetApproach_InvalidValue(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := sourcesRun([]string{"set-approach", "test", "invalid"}, &buf)
	if err == nil {
		t.Error("expected error for invalid approach value")
	}
}

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

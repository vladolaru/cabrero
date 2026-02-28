package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

func TestBlocklist_List_Empty(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := blocklistRun([]string{"list"}, &buf)
	if err != nil {
		t.Fatalf("blocklist list: %v", err)
	}
	if !strings.Contains(buf.String(), "empty") && !strings.Contains(buf.String(), "0") {
		t.Error("expected empty blocklist indication")
	}
}

func TestBlocklist_AddAndList(t *testing.T) {
	setupConfigTest(t)

	var buf bytes.Buffer
	err := blocklistRun([]string{"add", "sess-test-1234"}, &buf)
	if err != nil {
		t.Fatalf("blocklist add: %v", err)
	}

	buf.Reset()
	err = blocklistRun([]string{"list"}, &buf)
	if err != nil {
		t.Fatalf("blocklist list: %v", err)
	}
	if !strings.Contains(buf.String(), "sess-test-1234") {
		t.Error("expected added session in blocklist")
	}
}

func TestBlocklist_Remove(t *testing.T) {
	setupConfigTest(t)

	store.BlockSession("sess-remove-me", time.Now())

	var buf bytes.Buffer
	err := blocklistRun([]string{"remove", "sess-remove-me"}, &buf)
	if err != nil {
		t.Fatalf("blocklist remove: %v", err)
	}

	if store.IsBlocked("sess-remove-me") {
		t.Error("session should have been unblocked")
	}
}

func TestBlocklist_Remove_NotFound(t *testing.T) {
	setupConfigTest(t)

	var buf bytes.Buffer
	err := blocklistRun([]string{"remove", "nonexistent-session"}, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "was not in the blocklist") {
		t.Error("expected 'was not in the blocklist' message")
	}
}

func TestBlocklist_Add_AlreadyBlocked(t *testing.T) {
	setupConfigTest(t)

	store.BlockSession("sess-duplicate", time.Now())

	var buf bytes.Buffer
	err := blocklistRun([]string{"add", "sess-duplicate"}, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "already blocked") {
		t.Error("expected 'already blocked' message")
	}
}

func TestBlocklist_NoSubcommand(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := blocklistRun(nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Usage") {
		t.Error("expected usage help")
	}
}

func TestBlocklist_Add_MissingArg(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := blocklistRun([]string{"add"}, &buf)
	if err == nil {
		t.Error("expected error for missing session_id argument")
	}
}

func TestBlocklist_Remove_MissingArg(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := blocklistRun([]string{"remove"}, &buf)
	if err == nil {
		t.Error("expected error for missing session_id argument")
	}
}

func TestBlocklist_UnknownSubcommand(t *testing.T) {
	setupConfigTest(t)
	var buf bytes.Buffer
	err := blocklistRun([]string{"bogus"}, &buf)
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}
}

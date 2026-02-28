package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/store"
)

// Blocklist manages the session blocklist.
func Blocklist(args []string) error {
	return blocklistRun(args, os.Stdout)
}

func blocklistRun(args []string, w io.Writer) error {
	if len(args) == 0 {
		return blocklistHelp(w)
	}

	switch args[0] {
	case "list":
		return blocklistList(args[1:], w)
	case "add":
		return blocklistAdd(args[1:], w)
	case "remove":
		return blocklistRemove(args[1:], w)
	case "help", "--help", "-h":
		return blocklistHelp(w)
	default:
		return fmt.Errorf("unknown blocklist subcommand: %q", args[0])
	}
}

func blocklistList(args []string, w io.Writer) error {
	entries, err := store.ReadBlocklistEntries()
	if err != nil {
		return fmt.Errorf("reading blocklist: %w", err)
	}

	if len(entries) == 0 {
		fmt.Fprintln(w, "Blocklist is empty (0 entries).")
		return nil
	}

	type entry struct {
		id        string
		blockedAt time.Time
	}
	var sorted []entry
	for id, e := range entries {
		sorted = append(sorted, entry{id, e.BlockedAt})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].blockedAt.After(sorted[j].blockedAt)
	})

	limit := 50
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	fmt.Fprintf(w, "%-40s  %s\n", "SESSION ID", "BLOCKED")
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────")

	for _, e := range sorted {
		blocked := cli.RelativeTime(e.blockedAt)
		if e.blockedAt.IsZero() {
			blocked = "unknown"
		}
		fmt.Fprintf(w, "%-40s  %s\n", e.id, blocked)
	}

	fmt.Fprintf(w, "\nShowing %d of %d blocked sessions.\n", len(sorted), len(entries))
	return nil
}

func blocklistAdd(args []string, w io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero blocklist add <session_id>")
	}
	sessionID := args[0]

	if store.IsBlocked(sessionID) {
		fmt.Fprintf(w, "Session %s is already blocked.\n", sessionID)
		return nil
	}

	if err := store.BlockSession(sessionID, time.Now()); err != nil {
		return fmt.Errorf("blocking session: %w", err)
	}
	fmt.Fprintf(w, "Session %s added to blocklist.\n", sessionID)
	return nil
}

func blocklistRemove(args []string, w io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero blocklist remove <session_id>")
	}
	sessionID := args[0]

	removed, err := store.UnblockSession(sessionID)
	if err != nil {
		return fmt.Errorf("unblocking session: %w", err)
	}
	if !removed {
		fmt.Fprintf(w, "Session %s was not in the blocklist.\n", sessionID)
		return nil
	}
	fmt.Fprintf(w, "Session %s removed from blocklist.\n", sessionID)
	return nil
}

func blocklistHelp(w io.Writer) error {
	fmt.Fprintln(w, "Usage: cabrero blocklist <subcommand> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  list                  Show blocked sessions")
	fmt.Fprintln(w, "  add <session_id>      Block a session from processing")
	fmt.Fprintln(w, "  remove <session_id>   Unblock a session")
	return nil
}

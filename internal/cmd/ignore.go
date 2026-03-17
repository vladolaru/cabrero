package cmd

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/store"
)

// Ignore manages the project ignore list.
func Ignore(args []string) error {
	return ignoreRun(args, os.Stdout)
}

func ignoreRun(args []string, w io.Writer) error {
	if len(args) == 0 {
		return ignoreHelp(w)
	}

	switch args[0] {
	case "list":
		return ignoreList(w)
	case "add":
		return ignoreAdd(args[1:], w)
	case "remove":
		return ignoreRemove(args[1:], w)
	case "clean":
		return ignoreClean(args[1:], w)
	case "help", "--help", "-h":
		return ignoreHelp(w)
	default:
		return fmt.Errorf("unknown ignore subcommand: %q", args[0])
	}
}

func ignoreList(w io.Writer) error {
	patterns, err := store.ReadIgnoredPatterns()
	if err != nil {
		return fmt.Errorf("reading ignored patterns: %w", err)
	}
	if len(patterns) == 0 {
		fmt.Fprintln(w, "No ignored project patterns.")
		return nil
	}

	fmt.Fprintf(w, "%-30s  %s\n", "PATTERN", "ADDED")
	fmt.Fprintln(w, cli.Accent("──────────────────────────────────────────────"))

	for _, p := range patterns {
		added := cli.RelativeTime(p.AddedAt)
		if p.AddedAt.IsZero() {
			added = "unknown"
		}
		fmt.Fprintf(w, "%-30s  %s\n", p.Pattern, added)
	}

	fmt.Fprintf(w, "\n%d pattern(s).\n", len(patterns))
	return nil
}

func ignoreAdd(args []string, w io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero ignore add <pattern>")
	}
	pattern := args[0]

	// Check if already present.
	patterns, err := store.ReadIgnoredPatterns()
	if err != nil {
		return err
	}
	for _, p := range patterns {
		if strings.EqualFold(p.Pattern, pattern) {
			fmt.Fprintf(w, "Pattern %q is already ignored.\n", pattern)
			return nil
		}
	}

	if err := store.AddIgnoredPattern(pattern); err != nil {
		return fmt.Errorf("adding pattern: %w", err)
	}
	fmt.Fprintf(w, "Added pattern %q to ignore list.\n", pattern)

	// Show how many existing sessions match.
	count := store.CountIgnoredSessions()
	if count > 0 {
		fmt.Fprintf(w, "\n%d existing session(s) match ignored patterns.\n", count)
		fmt.Fprintln(w, "Run 'cabrero ignore clean' to remove them.")
	}
	return nil
}

func ignoreRemove(args []string, w io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero ignore remove <pattern>")
	}
	pattern := args[0]

	removed, err := store.RemoveIgnoredPattern(pattern)
	if err != nil {
		return fmt.Errorf("removing pattern: %w", err)
	}
	if !removed {
		fmt.Fprintf(w, "Pattern %q is not in the ignore list.\n", pattern)
		return nil
	}
	fmt.Fprintf(w, "Removed pattern %q from ignore list.\n", pattern)
	return nil
}

func ignoreClean(args []string, w io.Writer) error {
	dryRun := slices.Contains(args, "--dry-run")

	sessions, err := store.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	var matching []store.Metadata
	for _, s := range sessions {
		if store.IsProjectIgnored(s.Project) {
			matching = append(matching, s)
		}
	}

	if len(matching) == 0 {
		fmt.Fprintln(w, "No sessions match ignored patterns.")
		return nil
	}

	if dryRun {
		fmt.Fprintf(w, "Would remove %d session(s):\n", len(matching))
		for _, s := range matching {
			fmt.Fprintf(w, "  %s  %s\n", store.ShortSessionID(s.SessionID), store.ProjectDisplayName(s.Project))
		}
		return nil
	}

	removed, err := store.CleanIgnoredSessions()
	if err != nil {
		return fmt.Errorf("cleaning sessions: %w", err)
	}
	fmt.Fprintf(w, "Removed %d session(s) matching ignored patterns.\n", removed)
	return nil
}

func ignoreHelp(w io.Writer) error {
	fmt.Fprintln(w, "Usage: cabrero ignore <subcommand> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  list                  Show ignored project patterns")
	fmt.Fprintln(w, "  add <pattern>         Ignore projects matching substring (case-insensitive)")
	fmt.Fprintln(w, "  remove <pattern>      Stop ignoring a pattern")
	fmt.Fprintln(w, "  clean [--dry-run]     Remove existing sessions from ignored projects")
	return nil
}

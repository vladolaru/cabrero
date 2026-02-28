package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/store"
)

// Sources manages the source registry.
func Sources(args []string) error {
	return sourcesRun(args, os.Stdout)
}

func sourcesRun(args []string, w io.Writer) error {
	if len(args) == 0 {
		return sourcesHelp(w)
	}

	switch args[0] {
	case "list":
		return sourcesList(w)
	case "set-ownership":
		return sourcesSetOwnership(args[1:], w)
	case "set-approach":
		return sourcesSetApproach(args[1:], w)
	case "help", "--help", "-h":
		return sourcesHelp(w)
	default:
		return fmt.Errorf("unknown sources subcommand: %q", args[0])
	}
}

func sourcesList(w io.Writer) error {
	sources, err := store.ReadSources()
	if err != nil {
		return fmt.Errorf("reading sources: %w", err)
	}

	if len(sources) == 0 {
		fmt.Fprintln(w, "No sources tracked yet.")
		return nil
	}

	fmt.Fprintf(w, "%-30s  %-25s  %-10s  %-10s  %s\n",
		"NAME", "ORIGIN", "OWNERSHIP", "APPROACH", "SESSIONS")
	fmt.Fprintln(w, "────────────────────────────────────────────────────────────────────────────────────────────────")

	for _, s := range sources {
		name := s.Name
		if len(name) > 30 {
			name = "…" + name[len(name)-29:]
		}
		origin := s.Origin
		if len(origin) > 25 {
			origin = "…" + origin[len(origin)-24:]
		}
		ownership := s.Ownership
		if ownership == "" {
			ownership = "-"
		}
		approach := s.Approach
		if approach == "" {
			approach = "-"
		}
		fmt.Fprintf(w, "%-30s  %-25s  %-10s  %-10s  %d\n",
			name, origin, ownership, approach, s.SessionCount)
	}

	fmt.Fprintf(w, "\n%d sources.\n", len(sources))
	return nil
}

func sourcesSetOwnership(args []string, w io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: cabrero sources set-ownership <name> <mine|not_mine>")
	}
	name := args[0]
	value := args[1]

	if value != "mine" && value != "not_mine" {
		return fmt.Errorf("invalid ownership %q (must be mine or not_mine)", value)
	}

	sources, err := store.ReadSources()
	if err != nil {
		return err
	}

	var matched int
	var matchOrigin string
	for _, s := range sources {
		if s.Name == name {
			matched++
			matchOrigin = s.Origin
		}
	}

	if matched == 0 {
		return fmt.Errorf("source %q not found", name)
	}

	if matched > 1 {
		return fmt.Errorf("multiple sources named %q — specify origin", name)
	}

	now := time.Now()
	err = store.UpdateSourceByIdentity(name, matchOrigin, func(s *fitness.Source) {
		s.Ownership = value
		s.ClassifiedAt = &now
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Source %q ownership set to %q.\n", name, value)
	return nil
}

func sourcesSetApproach(args []string, w io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: cabrero sources set-approach <name> <iterate|evaluate|paused>")
	}
	name := args[0]
	value := args[1]

	if value != "iterate" && value != "evaluate" && value != "paused" {
		return fmt.Errorf("invalid approach %q (must be iterate, evaluate, or paused)", value)
	}

	sources, err := store.ReadSources()
	if err != nil {
		return err
	}

	var matched int
	var matchOrigin string
	for _, s := range sources {
		if s.Name == name {
			matched++
			matchOrigin = s.Origin
		}
	}

	if matched == 0 {
		return fmt.Errorf("source %q not found", name)
	}

	if matched > 1 {
		return fmt.Errorf("multiple sources named %q — specify origin", name)
	}

	err = store.UpdateSourceByIdentity(name, matchOrigin, func(s *fitness.Source) {
		s.Approach = value
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Source %q approach set to %q.\n", name, value)
	return nil
}

func sourcesHelp(w io.Writer) error {
	fmt.Fprintln(w, "Usage: cabrero sources <subcommand> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  list                                   Show all tracked sources")
	fmt.Fprintln(w, "  set-ownership <name> <mine|not_mine>   Set source ownership")
	fmt.Fprintln(w, "  set-approach <name> <iterate|evaluate|paused>  Set source approach")
	return nil
}

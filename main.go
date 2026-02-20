package main

import (
	"fmt"
	"os"

	"github.com/vladolaru/cabrero/internal/cmd"
	"github.com/vladolaru/cabrero/internal/store"
)

var version = "dev"

type command struct {
	name string
	desc string
	run  func(args []string) error
}

var commands = []command{
	{"run", "Run the full pipeline on a session", cmd.Run},
	{"sessions", "List captured sessions", cmd.Sessions},
	{"status", "Show pipeline health and store overview", cmd.Status},
	{"proposals", "List pending proposals", cmd.Proposals},
	{"inspect", "Show a proposal with full citation chain", cmd.Inspect},
	{"approve", "Approve and apply a proposal", cmdNotImplemented},
	{"reject", "Reject a proposal with optional reason", cmdNotImplemented},
	{"replay", "Re-run pipeline with a different prompt", cmdNotImplemented},
	{"prompts", "List prompt files with versions", cmdNotImplemented},
	{"import", "Seed the store from existing CC session files", cmd.Import},
	{"backfill", "Run pipeline on existing sessions with date/project filtering", cmd.Backfill},
	{"daemon", "Run background session processor (for launchd)", cmd.Daemon},
	{"setup", "Install and configure Cabrero", cmdSetup},
	{"update", "Update Cabrero to latest release", cmdUpdate},
	{"doctor", "Diagnose issues and auto-fix problems", cmdDoctor},
	{"uninstall", "Remove Cabrero from this system", cmd.Uninstall},
}

func main() {
	// Initialize store on every invocation.
	if err := store.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing store: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	sub := os.Args[1]

	if sub == "help" || sub == "--help" || sub == "-h" {
		printHelp()
		os.Exit(0)
	}

	if sub == "version" || sub == "--version" {
		fmt.Printf("cabrero %s\n", version)
		os.Exit(0)
	}

	for _, c := range commands {
		if c.name == sub {
			if err := c.run(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'cabrero help' for usage.\n", sub)
	os.Exit(1)
}

func printHelp() {
	fmt.Printf("cabrero %s — CC auto-improvement system\n\n", version)
	fmt.Println("Usage: cabrero <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	maxLen := 0
	for _, c := range commands {
		if len(c.name) > maxLen {
			maxLen = len(c.name)
		}
	}
	for _, c := range commands {
		fmt.Printf("  %-*s  %s\n", maxLen, c.name, c.desc)
	}
	fmt.Println()
	fmt.Println("Run 'cabrero help' for this message.")
}

func cmdSetup(args []string) error {
	return cmd.Setup(args, cmd.EmbeddedHooks{
		PreCompact: preCompactHookScript,
		SessionEnd: sessionEndHookScript,
	})
}

func cmdUpdate(args []string) error {
	return cmd.Update(args, version, cmd.EmbeddedHooks{
		PreCompact: preCompactHookScript,
		SessionEnd: sessionEndHookScript,
	})
}

func cmdDoctor(args []string) error {
	return cmd.Doctor(args, cmd.EmbeddedHooks{
		PreCompact: preCompactHookScript,
		SessionEnd: sessionEndHookScript,
	})
}

func cmdNotImplemented(args []string) error {
	fmt.Println("Not yet implemented.")
	return nil
}

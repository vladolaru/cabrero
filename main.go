package main

import (
	"fmt"
	"os"
)

const version = "0.1.0-dev"

type command struct {
	name  string
	desc  string
	run   func(args []string) error
}

var commands = []command{
	{"run", "Run the full pipeline on a session", cmdNotImplemented},
	{"sessions", "List captured sessions", cmdNotImplemented},
	{"status", "Show pipeline health and store overview", cmdNotImplemented},
	{"proposals", "List pending proposals", cmdNotImplemented},
	{"inspect", "Show a proposal with full citation chain", cmdNotImplemented},
	{"approve", "Approve and apply a proposal", cmdNotImplemented},
	{"reject", "Reject a proposal with optional reason", cmdNotImplemented},
	{"replay", "Re-run pipeline with a different prompt", cmdNotImplemented},
	{"prompts", "List prompt files with versions", cmdNotImplemented},
	{"import", "Seed the store from existing CC session files", cmdNotImplemented},
}

func main() {
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

	for _, cmd := range commands {
		if cmd.name == sub {
			if err := cmd.run(os.Args[2:]); err != nil {
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
	for _, cmd := range commands {
		if len(cmd.name) > maxLen {
			maxLen = len(cmd.name)
		}
	}
	for _, cmd := range commands {
		fmt.Printf("  %-*s  %s\n", maxLen, cmd.name, cmd.desc)
	}
	fmt.Println()
	fmt.Println("Run 'cabrero help' for this message.")
}

func cmdNotImplemented(args []string) error {
	fmt.Println("Not yet implemented.")
	return nil
}

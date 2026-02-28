package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/store"
)

// Config is the entry point for `cabrero config`.
func Config(args []string) error {
	return configRun(args, os.Stdout)
}

func configRun(args []string, w io.Writer) error {
	if len(args) == 0 {
		return configHelp(w)
	}

	switch args[0] {
	case "get":
		return configGet(args[1:], w)
	case "set":
		return configSet(args[1:])
	case "unset":
		return configUnset(args[1:])
	case "list":
		return configList(args[1:], w)
	case "help", "--help", "-h":
		return configHelp(w)
	default:
		return fmt.Errorf("unknown config subcommand: %q\nRun 'cabrero config help' for usage", args[0])
	}
}

func configGet(args []string, w io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero config get <key>")
	}
	val, _, err := store.ConfigGet(args[0])
	if err != nil {
		return err
	}
	fmt.Fprintln(w, val)
	return nil
}

func configSet(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: cabrero config set <key> <value>")
	}
	return store.ConfigSet(args[0], args[1])
}

func configUnset(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero config unset <key>")
	}
	return store.ConfigUnset(args[0])
}

func configList(args []string, w io.Writer) error {
	showDefaults := false
	for _, a := range args {
		if a == "--defaults" {
			showDefaults = true
		}
	}
	entries, err := store.ConfigList()
	if err != nil {
		return err
	}

	maxLen := 0
	for _, e := range entries {
		if len(e.Key) > maxLen {
			maxLen = len(e.Key)
		}
	}

	for _, e := range entries {
		line := fmt.Sprintf("%-*s = %s", maxLen, e.Key, e.Value)
		if showDefaults && e.IsDefault {
			line += " " + cli.Muted("(default)")
		}
		fmt.Fprintln(w, line)
	}
	return nil
}

func configHelp(w io.Writer) error {
	fmt.Fprintln(w, "Usage: cabrero config <subcommand> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  get <key>          Read a config value")
	fmt.Fprintln(w, "  set <key> <value>  Set a config value")
	fmt.Fprintln(w, "  unset <key>        Remove override (revert to default)")
	fmt.Fprintln(w, "  list [--defaults]  Show all config values")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Keys:")

	keys := store.ConfigKeys()
	maxLen := 0
	for _, k := range keys {
		if len(k.Key) > maxLen {
			maxLen = len(k.Key)
		}
	}
	for _, k := range keys {
		fmt.Fprintf(w, "  %-*s  %s %s\n", maxLen, k.Key, k.Description,
			cli.Muted(fmt.Sprintf("[default: %s]", k.DefaultValue)))
	}
	return nil
}

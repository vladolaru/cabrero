package cmd

import (
	"flag"
	"fmt"

	"github.com/vladolaru/cabrero/internal/store"
)

// Calibrate dispatches to tag, untag, or list subcommands for managing
// the calibration set used in prompt testing.
func Calibrate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cabrero calibrate <tag|untag|list> [options]")
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "tag":
		return calibrateTag(rest)
	case "untag":
		return calibrateUntag(rest)
	case "list":
		return calibrateList(rest)
	default:
		return fmt.Errorf("unknown subcommand %q: use tag, untag, or list", sub)
	}
}

func calibrateTag(args []string) error {
	fs := flag.NewFlagSet("calibrate tag", flag.ContinueOnError)
	label := fs.String("label", "", "label: approve or reject")
	note := fs.String("note", "", "optional note")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: cabrero calibrate tag <session_id> --label approve|reject [--note \"text\"]")
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: cabrero calibrate tag <session_id> --label approve|reject [--note \"text\"]")
	}
	sessionID := fs.Arg(0)

	if *label == "" {
		return fmt.Errorf("--label is required (approve or reject)")
	}

	// Validate session exists in the store.
	if !store.SessionExists(sessionID) {
		return fmt.Errorf("session %q not found in store", sessionID)
	}

	entry := store.CalibrationEntry{
		SessionID: sessionID,
		Label:     *label,
		Note:      *note,
	}
	if err := store.AddCalibrationEntry(entry); err != nil {
		return err
	}

	fmt.Printf("Tagged session %s as %s.\n", sessionID, *label)
	return nil
}

func calibrateUntag(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero calibrate untag <session_id>")
	}
	sessionID := args[0]

	if err := store.RemoveCalibrationEntry(sessionID); err != nil {
		return err
	}

	fmt.Printf("Untagged session %s.\n", sessionID)
	return nil
}

func calibrateList(_ []string) error {
	entries, err := store.ListCalibrationEntries()
	if err != nil {
		return fmt.Errorf("reading calibration set: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("Calibration set is empty.")
		fmt.Println("Use 'cabrero calibrate tag <session_id> --label approve|reject' to add entries.")
		return nil
	}

	fmt.Printf("%-18s  %-8s  %-10s  %s\n", "SESSION", "LABEL", "TAGGED", "NOTE")
	fmt.Println("────────────────────────────────────────────────────────────────────────────")

	for _, e := range entries {
		sid := e.SessionID
		if len(sid) > 18 {
			sid = sid[:18]
		}

		age := formatAge(e.TaggedAt)

		note := e.Note
		if len(note) > 40 {
			note = note[:37] + "..."
		}

		fmt.Printf("%-18s  %-8s  %-10s  %s\n", sid, e.Label, age, note)
	}

	fmt.Printf("\n%d entries in calibration set.\n", len(entries))
	return nil
}

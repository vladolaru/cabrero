package cmd

import (
	"flag"
	"fmt"

	"github.com/vladolaru/cabrero/internal/apply"
	"github.com/vladolaru/cabrero/internal/store"
)

// RollbackCmd restores a file to its pre-change content.
func RollbackCmd(args []string) error {
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("usage: cabrero rollback <change_id> [--yes]")
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: cabrero rollback <change_id> [--yes]")
	}
	changeID := fs.Arg(0)

	// Look up the change to show a summary before confirming.
	entry, err := store.GetChange(changeID)
	if err != nil {
		return fmt.Errorf("reading change: %w", err)
	}
	if entry == nil {
		return fmt.Errorf("change %q not found", changeID)
	}

	fmt.Printf("Change:   %s\n", entry.ID)
	fmt.Printf("Source:   %s\n", entry.SourceName)
	fmt.Printf("File:     %s\n", entry.FilePath)
	fmt.Printf("Applied:  %s\n", entry.Timestamp.Local().Format("2006-01-02 15:04"))
	fmt.Println()

	if !*yes {
		if !promptYesNo("Rollback this change? The file will be restored to its previous content.") {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	path, err := apply.Rollback(changeID)
	if err != nil {
		return err
	}

	fmt.Printf("Rolled back. File restored: %s\n", path)
	return nil
}

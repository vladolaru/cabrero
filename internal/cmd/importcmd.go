package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vladolaru/cabrero/internal/store"
)

// Import seeds the store from existing CC session files.
func Import(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	from := fs.String("from", "", "path to CC projects directory (e.g. ~/.claude/projects/)")
	dryRun := fs.Bool("dry-run", false, "preview what would be imported without copying")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *from == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		defaultPath := filepath.Join(home, ".claude", "projects")
		from = &defaultPath
	}

	// Expand ~ if present.
	if strings.HasPrefix(*from, "~/") {
		home, _ := os.UserHomeDir()
		expanded := filepath.Join(home, (*from)[2:])
		from = &expanded
	}

	info, err := os.Stat(*from)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", *from, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", *from)
	}

	if *dryRun {
		fmt.Println("Dry run — no files will be copied.")
		fmt.Println()
	}

	imported := 0
	skipped := 0

	// Walk the projects directory looking for *.jsonl files.
	err = filepath.Walk(*from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".jsonl") {
			return nil
		}

		// Session ID is the filename without .jsonl extension.
		sessionID := strings.TrimSuffix(info.Name(), ".jsonl")

		if sessionID == "" || len(sessionID) < 8 {
			return nil
		}

		// Skip if already in store or blocklist.
		if store.SessionExists(sessionID) {
			skipped++
			return nil
		}
		if store.IsBlocked(sessionID) {
			skipped++
			return nil
		}

		// Extract project slug from parent directory name.
		project := filepath.Base(filepath.Dir(path))
		if project == filepath.Base(*from) {
			// File is directly in the root of --from, no project.
			project = ""
		}

		// Use the file's modification time as the session timestamp.
		modTime := info.ModTime()

		if *dryRun {
			display := store.ProjectDisplayName(project)
			if display == "" {
				display = "(none)"
			}
			fmt.Printf("  Would import: %s  [%s]  %s\n", sessionID, display, modTime.Format("2006-01-02 15:04"))
			imported++
			return nil
		}

		if err := store.WriteSession(sessionID, path, "imported", "", modTime, project); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to import %s: %v\n", sessionID, err)
			return nil
		}
		imported++
		return nil
	})
	if err != nil {
		return fmt.Errorf("scanning %s: %w", *from, err)
	}

	fmt.Println()
	if *dryRun {
		fmt.Printf("Would import %d sessions, skipped %d (already present).\n", imported, skipped)
	} else {
		fmt.Printf("Imported %d sessions, skipped %d (already present).\n", imported, skipped)
	}
	return nil
}

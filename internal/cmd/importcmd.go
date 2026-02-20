package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vladolaru/cabrero/internal/parser"
	"github.com/vladolaru/cabrero/internal/store"
)

// ImportResult holds the outcome of an import operation.
type ImportResult struct {
	Imported int
	Skipped  int
}

// RunImport executes the import logic. When quiet is true, per-session output is suppressed.
func RunImport(from string, dryRun bool, quiet bool) (ImportResult, error) {
	info, err := os.Stat(from)
	if err != nil {
		return ImportResult{}, fmt.Errorf("cannot access %s: %w", from, err)
	}
	if !info.IsDir() {
		return ImportResult{}, fmt.Errorf("%s is not a directory", from)
	}

	if dryRun && !quiet {
		fmt.Println("Dry run — no files will be copied.")
		fmt.Println()
	}

	result := ImportResult{}

	// Walk the projects directory looking for *.jsonl files.
	err = filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
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
			result.Skipped++
			return nil
		}
		if store.IsBlocked(sessionID) {
			result.Skipped++
			return nil
		}

		// Extract project slug from parent directory name.
		project := filepath.Base(filepath.Dir(path))
		if project == filepath.Base(from) {
			// File is directly in the root of from, no project.
			project = ""
		}

		// Use the file's modification time as the session timestamp.
		modTime := info.ModTime()

		if dryRun {
			if !quiet {
				display := store.ProjectDisplayName(project)
				if display == "" {
					display = "(none)"
				}
				fmt.Printf("  Would import: %s  [%s]  %s\n", sessionID, display, modTime.Format("2006-01-02 15:04"))
			}
			result.Imported++
			return nil
		}

		if err := store.WriteSession(sessionID, path, "imported", "", modTime, project); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to import %s: %v\n", sessionID, err)
			return nil
		}

		// Run pre-parser to generate digest (cheap, no LLM).
		digest, parseErr := parser.ParseSession(sessionID)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "  Warning: pre-parser failed for %s: %v\n", sessionID, parseErr)
		} else {
			if writeErr := parser.WriteDigest(digest); writeErr != nil {
				fmt.Fprintf(os.Stderr, "  Warning: writing digest for %s: %v\n", sessionID, writeErr)
			}
		}

		result.Imported++
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("scanning %s: %w", from, err)
	}

	return result, nil
}

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

	result, err := RunImport(*from, *dryRun, false)
	if err != nil {
		return err
	}

	fmt.Println()
	if *dryRun {
		fmt.Printf("Would import %d sessions (with pre-parsing), skipped %d (already present).\n", result.Imported, result.Skipped)
	} else {
		fmt.Printf("Imported %d sessions (with digests), skipped %d (already present).\n", result.Imported, result.Skipped)
	}
	return nil
}

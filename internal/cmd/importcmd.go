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
//
// Only reads two known levels where JSONL files exist:
//   - <project>/<session>.jsonl
//   - <project>/<session>/subagents/<agent>.jsonl
//
// This avoids descending into tool-results/ and other subdirectories that can
// trigger macOS network volume access prompts.
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

	// Read blocklist once to avoid N file reads inside the loop.
	blocked, err := store.ReadBlocklist()
	if err != nil {
		return result, fmt.Errorf("reading blocklist: %w", err)
	}

	projects, err := os.ReadDir(from)
	if err != nil {
		return result, fmt.Errorf("reading %s: %w", from, err)
	}

	for _, proj := range projects {
		if !proj.IsDir() {
			continue
		}
		projectDir := filepath.Join(from, proj.Name())

		entries, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				// Check for subagents/ inside session directories.
				importSubagents(filepath.Join(projectDir, entry.Name()), proj.Name(), from, dryRun, quiet, blocked, &result)
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			importSession(filepath.Join(projectDir, entry.Name()), entry, proj.Name(), from, dryRun, quiet, blocked, &result)
		}
	}

	return result, nil
}

// importSubagents checks <sessionDir>/subagents/ for agent JSONL files.
func importSubagents(sessionDir, project, from string, dryRun, quiet bool, blocked map[string]bool, result *ImportResult) {
	subDir := filepath.Join(sessionDir, "subagents")
	entries, err := os.ReadDir(subDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		importSession(filepath.Join(subDir, entry.Name()), entry, project, from, dryRun, quiet, blocked, result)
	}
}

func importSession(path string, entry os.DirEntry, project, from string, dryRun, quiet bool, blocked map[string]bool, result *ImportResult) {
	sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
	if sessionID == "" || len(sessionID) < 8 {
		return
	}

	if store.SessionExists(sessionID) {
		result.Skipped++
		return
	}
	if blocked[sessionID] {
		result.Skipped++
		return
	}

	info, err := entry.Info()
	if err != nil {
		return
	}
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
		return
	}

	if err := store.WriteSession(sessionID, path, "imported", "", modTime, project); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to import %s: %v\n", sessionID, err)
		return
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

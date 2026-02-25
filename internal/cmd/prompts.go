package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/vladolaru/cabrero/internal/cli"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Prompts lists prompt files from ~/.cabrero/prompts/ with name, version,
// file path, and last-modified time.
func Prompts(args []string) error {
	versions, err := pipeline.ListPromptVersions()
	if err != nil {
		return fmt.Errorf("reading prompts: %w", err)
	}

	if len(versions) == 0 {
		fmt.Println("No prompt files found in", filepath.Join(store.Root(), "prompts")+"/")
		fmt.Println("Prompt files are created automatically when running the pipeline.")
		return nil
	}

	fmt.Printf("%-20s  %-8s  %-14s  %s\n", "NAME", "VERSION", "LAST MODIFIED", "PATH")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────────────")

	promptsDir := filepath.Join(store.Root(), "prompts")

	for _, v := range versions {
		age := cli.RelativeTime(v.UpdatedAt)

		filename := v.Name
		if v.Version != "" {
			filename += "-" + v.Version
		}
		filename += ".txt"
		path := filepath.Join(promptsDir, filename)

		fmt.Printf("%-20s  %-8s  %-14s  %s\n", v.Name, v.Version, age, path)
	}

	fmt.Printf("\n%d prompt files.\n", len(versions))
	return nil
}


package pipeline

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vladolaru/cabrero/internal/store"
)

// claudeConfig controls how the claude CLI is invoked.
type claudeConfig struct {
	Model        string // model name (e.g. "claude-haiku-4-5", "claude-sonnet-4-6")
	SystemPrompt string // system prompt text passed via --system-prompt
	Effort       string // reasoning effort ("" for default, "high" for Sonnet)
}

// invokeClaude runs the claude CLI with the given config and data on stdin.
// The system prompt is passed via --system-prompt, keeping it separate from the
// data which is piped through stdin.
func invokeClaude(cfg claudeConfig, stdin io.Reader) (string, error) {
	args := []string{
		"--model", cfg.Model,
		"--print",
		"--system-prompt", cfg.SystemPrompt,
		"--no-session-persistence",
		"--disable-slash-commands",
		"--tools", "",
	}

	if cfg.Effort != "" {
		args = append(args, "--effort", cfg.Effort)
	}

	cmd := exec.Command("claude", args...)
	cmd.Env = append(os.Environ(), "CABRERO_SESSION=1")
	cmd.Stdin = stdin

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude exited with code %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("running claude: %w", err)
	}

	return string(out), nil
}

// readPromptTemplate reads a prompt file from ~/.cabrero/prompts/.
func readPromptTemplate(filename string) (string, error) {
	path := filepath.Join(store.Root(), "prompts", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("prompt file not found: %s\nRun 'cabrero prompts' or create it manually", path)
	}
	return string(data), nil
}

// cleanLLMJSON strips markdown code fences and whitespace from LLM output.
func cleanLLMJSON(raw string) string {
	s := strings.TrimSpace(raw)

	// Strip markdown fences: ```json ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		// Find end of first line (may contain ```json or just ```)
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		// Strip trailing ```
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}

	return s
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

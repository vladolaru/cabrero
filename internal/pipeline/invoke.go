package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// claudeConfig controls how the claude CLI is invoked.
type claudeConfig struct {
	Model        string // model name (e.g. "claude-haiku-4-5", "claude-sonnet-4-6")
	SystemPrompt string // system prompt text passed via --system-prompt
	Effort       string // reasoning effort ("" for default, "high" for Evaluator)
	// Agentic mode fields (ignored when Agentic is false).
	Agentic      bool          // true = use -p with tools; false = use --print with stdin
	Prompt       string        // user prompt (agentic mode: positional arg; print mode: ignored)
	AllowedTools string        // comma-separated tool names for --allowedTools
	MaxTurns     int           // turn budget — no --max-turns CLI flag exists; callers embed this in the prompt
	Timeout      time.Duration // hard wall-clock timeout via context.WithTimeout
	Stdin        io.Reader     // only used in --print mode (Agentic=false)
}

// invokeClaude runs the claude CLI with the given config.
//
// Two modes are supported:
//   - Print mode (Agentic=false): uses --print with stdin pipe and all tools disabled.
//     Data is provided via cfg.Stdin.
//   - Agentic mode (Agentic=true): uses -p with the prompt as a positional argument,
//     --allowedTools for selective tool access, and --output-format text for clean output.
//     MaxTurns is informational only (embedded in the prompt by callers, not a CLI flag).
func invokeClaude(cfg claudeConfig) (string, error) {
	// Validate required fields per mode.
	if cfg.Agentic && cfg.Prompt == "" {
		return "", fmt.Errorf("invokeClaude: Prompt is required for agentic mode")
	}
	if !cfg.Agentic && cfg.Stdin == nil {
		return "", fmt.Errorf("invokeClaude: Stdin is required for print mode")
	}

	var args []string

	if cfg.Agentic {
		// Agentic mode: prompt as positional arg, tools enabled.
		args = []string{
			"--model", cfg.Model,
			"-p", cfg.Prompt,
			"--system-prompt", cfg.SystemPrompt,
			"--output-format", "text",
			"--no-session-persistence",
			"--disable-slash-commands",
		}
		if cfg.AllowedTools != "" {
			args = append(args, "--allowedTools", cfg.AllowedTools)
		}
		if cfg.Effort != "" {
			args = append(args, "--effort", cfg.Effort)
		}
	} else {
		// Print mode: stdin pipe, all tools disabled.
		args = []string{
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
	}

	// Always create a context so we can detect timeout in error handling.
	var ctx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), cfg.Timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = append(os.Environ(), "CABRERO_SESSION=1")

	if !cfg.Agentic {
		cmd.Stdin = cfg.Stdin
	}

	out, err := cmd.Output()
	if err != nil {
		if cfg.Timeout > 0 && ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("claude timed out after %s", cfg.Timeout)
		}
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

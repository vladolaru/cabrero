package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

var (
	invokeSem     chan struct{}
	invokeSemOnce sync.Once
)

// InitInvokeSemaphore sets the maximum number of concurrent claude CLI
// invocations. Call once at startup (e.g. from daemon or CLI main).
// A limit of 0 means unlimited (no semaphore).
func InitInvokeSemaphore(limit int) {
	invokeSemOnce.Do(func() {
		if limit > 0 {
			invokeSem = make(chan struct{}, limit)
		}
	})
}

// TryAcquireInvokeSemaphore attempts to acquire a semaphore slot without
// blocking. Returns true if acquired (caller must call ReleaseInvokeSemaphore),
// false if all slots are busy. When no semaphore is configured, always returns true.
func TryAcquireInvokeSemaphore() bool {
	if invokeSem == nil {
		return true
	}
	select {
	case invokeSem <- struct{}{}:
		return true
	default:
		return false
	}
}

// ReleaseInvokeSemaphore releases a slot previously acquired via
// TryAcquireInvokeSemaphore. Must not be called without a matching acquire.
func ReleaseInvokeSemaphore() {
	if invokeSem != nil {
		<-invokeSem
	}
}

func acquireInvokeSemaphore() {
	if invokeSem != nil {
		invokeSem <- struct{}{}
	}
}

func releaseInvokeSemaphore() {
	ReleaseInvokeSemaphore()
}

// ResetInvokeSemaphoreForTest resets the semaphore for testing. Not thread-safe.
// Exported so daemon tests can reset state between test cases.
func ResetInvokeSemaphoreForTest() {
	invokeSem = nil
	invokeSemOnce = sync.Once{}
}

// resetInvokeSemaphore is the internal alias used by pipeline tests.
func resetInvokeSemaphore() {
	ResetInvokeSemaphoreForTest()
}

// ClaudeResult holds the parsed output from a claude CLI JSON response.
type ClaudeResult struct {
	Result              string  // LLM text output (from "result" field)
	SessionID           string  // CC session ID for cross-referencing
	NumTurns            int     // actual agentic turns used
	DurationMs          int     // total execution time (CC-reported)
	DurationApiMs       int     // API call time only (CC-reported)
	TotalCostUSD        float64 // total API cost
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	WebSearchRequests   int
	WebFetchRequests    int
	IsError             bool     // true if CC returned an error result
	Errors              []string // error messages from CC
}

// claudeJSONResponse is the raw JSON envelope from `claude --output-format json`.
type claudeJSONResponse struct {
	Type          string  `json:"type"`
	Subtype       string  `json:"subtype"`
	IsError       bool    `json:"is_error"`
	Result        string  `json:"result"`
	SessionID     string  `json:"session_id"`
	NumTurns      int     `json:"num_turns"`
	DurationMs    int     `json:"duration_ms"`
	DurationApiMs int     `json:"duration_api_ms"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	Usage         struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		ServerToolUse            struct {
			WebSearchRequests int `json:"web_search_requests"`
			WebFetchRequests  int `json:"web_fetch_requests"`
		} `json:"server_tool_use"`
	} `json:"usage"`
	Errors []string `json:"errors"`
}

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
	Debug        bool          // persist CC session transcript for inspection
	Logger       Logger        // for debug log messages (nil = no debug logging)
	// Isolation fields — restrict filesystem access and plugin loading.
	DisallowedTools string  // comma-separated deny rules for --disallowedTools
	PermissionMode  string  // permission mode (e.g. "dontAsk" to auto-deny unapproved tools)
	SettingSources  *string // setting sources to load (nil = default); avoid "" — broke in CLI 2.1.59+
}

// invokeClaude runs the claude CLI with the given config.
//
// Two modes are supported:
//   - Print mode (Agentic=false): uses --print with stdin pipe and all tools disabled.
//     Data is provided via cfg.Stdin. Returns the raw text output as ClaudeResult.Result.
//   - Agentic mode (Agentic=true): uses -p with the prompt as a positional argument,
//     --allowedTools for selective tool access, and --output-format json for structured output.
//     MaxTurns is informational only (embedded in the prompt by callers, not a CLI flag).
//
// When CC returns is_error: true, both a non-nil result (with usage data) and an error
// are returned so callers can capture partial usage from failed invocations.
func invokeClaude(cfg claudeConfig) (*ClaudeResult, error) {
	// Validate required fields per mode.
	if cfg.Agentic && cfg.Prompt == "" {
		return nil, fmt.Errorf("invokeClaude: Prompt is required for agentic mode")
	}
	if !cfg.Agentic && cfg.Stdin == nil {
		return nil, fmt.Errorf("invokeClaude: Stdin is required for print mode")
	}

	// Generate session ID and blocklist it for ALL agentic invocations.
	// Print-mode invocations skip this.
	var agenticSessionID string
	if cfg.Agentic {
		id, err := GenerateUUID()
		if err != nil {
			return nil, fmt.Errorf("generating session ID: %w", err)
		}
		agenticSessionID = id
		if err := store.BlockSession(agenticSessionID, time.Now()); err != nil {
			return nil, fmt.Errorf("blocklisting session: %w", err)
		}
		if cfg.Debug && cfg.Logger != nil {
			cfg.Logger.Info("  [debug] CC session %s persisted for inspection", agenticSessionID)
		}
	}

	args := buildClaudeArgs(cfg, agenticSessionID)

	if cfg.Debug && cfg.Logger != nil {
		cfg.Logger.Info("  [debug] claude args: %v", args)
	}

	// Acquire semaphore before starting the timeout so wait time in the
	// queue doesn't eat into the actual execution budget.
	if !TryAcquireInvokeSemaphore() {
		if cfg.Logger != nil {
			cfg.Logger.Info("  Waiting for a claude process slot (%d already running)...", cap(invokeSem))
		}
		acquireInvokeSemaphore()
	}

	// Create timeout context after acquiring the semaphore so the full
	// timeout applies to execution, not queuing.
	var ctx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), cfg.Timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = os.TempDir() // use temp dir — ~/.cabrero/ is one level below ~ and CC project discovery walks up, triggering macOS TCC prompts for ~/Desktop, ~/Music, etc.
	cmd.Env = cleanClaudeEnv()

	if !cfg.Agentic {
		cmd.Stdin = cfg.Stdin
	}

	out, err := cmd.Output()
	releaseInvokeSemaphore()
	if err != nil {
		if cfg.Timeout > 0 && ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude timed out after %s", cfg.Timeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Attempt to parse stdout for usage data even on non-zero exit.
			if cfg.Agentic && len(out) > 0 {
				if cr, parseErr := parseClaudeJSON(out); parseErr == nil {
					return cr, fmt.Errorf("claude exited with code %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
				}
			}
			return nil, fmt.Errorf("claude exited with code %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("running claude: %w", err)
	}

	// Print mode returns raw text — no JSON envelope.
	if !cfg.Agentic {
		return &ClaudeResult{Result: string(out)}, nil
	}

	// Agentic mode returns JSON envelope with usage data.
	cr, parseErr := parseClaudeJSON(out)
	if parseErr != nil {
		return nil, fmt.Errorf("parsing claude JSON output: %w", parseErr)
	}

	// If CC itself reported an error, return both result (for usage) and error.
	if cr.IsError {
		errMsg := "claude returned error"
		if len(cr.Errors) > 0 {
			errMsg = fmt.Sprintf("claude returned error: %s", strings.Join(cr.Errors, "; "))
		}
		return cr, fmt.Errorf("%s", errMsg)
	}

	return cr, nil
}

// buildClaudeArgs constructs the CLI argument list for the claude command.
// debugSessionID is only used when cfg.Debug is true.
func buildClaudeArgs(cfg claudeConfig, sessionID string) []string {
	var args []string

	if cfg.Agentic {
		// Agentic mode: prompt as positional arg, tools enabled.
		args = []string{
			"--model", cfg.Model,
			"-p", cfg.Prompt,
			"--system-prompt", cfg.SystemPrompt,
			"--output-format", "json",
			"--disable-slash-commands",
			"--mcp-config", `{"mcpServers":{}}`, // empty MCP config
			"--strict-mcp-config",               // ignore all other MCP sources (plugins, settings, project)
			"--settings", `{"disableAllHooks": true, "alwaysThinkingEnabled": false, "enabledPlugins": {}}`, // isolate from user settings: no hooks, no extended thinking, no plugins
		}
		// Agentic mode: always pass --session-id; never pass --no-session-persistence.
		// sessionID is always non-empty for agentic calls (generated by invokeClaude).
		if sessionID != "" {
			args = append(args, "--session-id", sessionID)
		}
		if cfg.AllowedTools != "" {
			args = append(args, "--allowedTools", cfg.AllowedTools)
		}
		if cfg.Effort != "" {
			args = append(args, "--effort", cfg.Effort)
		}
		if cfg.DisallowedTools != "" {
			args = append(args, "--disallowedTools", cfg.DisallowedTools)
		}
		if cfg.PermissionMode != "" {
			args = append(args, "--permission-mode", cfg.PermissionMode)
		}
		if cfg.SettingSources != nil {
			args = append(args, "--setting-sources", *cfg.SettingSources)
		}
	} else {
		// Print mode: stdin pipe, all tools disabled.
		args = []string{
			"--model", cfg.Model,
			"--print",
			"--system-prompt", cfg.SystemPrompt,
			"--disable-slash-commands",
			"--tools", "",
			"--mcp-config", `{"mcpServers":{}}`, // empty MCP config
			"--strict-mcp-config",               // ignore all other MCP sources (plugins, settings, project)
			"--settings", `{"disableAllHooks": true, "alwaysThinkingEnabled": false, "enabledPlugins": {}}`, // isolate from user settings: no hooks, no extended thinking, no plugins
		}
		if !cfg.Debug {
			args = append(args, "--no-session-persistence")
		} else {
			args = append(args, "--session-id", sessionID)
		}
		if cfg.Effort != "" {
			args = append(args, "--effort", cfg.Effort)
		}
		if cfg.PermissionMode != "" {
			args = append(args, "--permission-mode", cfg.PermissionMode)
		}
		if cfg.SettingSources != nil {
			args = append(args, "--setting-sources", *cfg.SettingSources)
		}
	}

	return args
}

// GenerateUUID returns a random UUID v4 string using crypto/rand.
func GenerateUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Set version (4) and variant (RFC 4122).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
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

// parseClaudeJSON parses the JSON envelope from `claude --output-format json`
// into a ClaudeResult.
func parseClaudeJSON(data []byte) (*ClaudeResult, error) {
	var resp claudeJSONResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("invalid claude JSON: %w", err)
	}

	return &ClaudeResult{
		Result:              resp.Result,
		SessionID:           resp.SessionID,
		NumTurns:            resp.NumTurns,
		DurationMs:          resp.DurationMs,
		DurationApiMs:       resp.DurationApiMs,
		TotalCostUSD:        resp.TotalCostUSD,
		InputTokens:         resp.Usage.InputTokens,
		OutputTokens:        resp.Usage.OutputTokens,
		CacheCreationTokens: resp.Usage.CacheCreationInputTokens,
		CacheReadTokens:     resp.Usage.CacheReadInputTokens,
		WebSearchRequests:   resp.Usage.ServerToolUse.WebSearchRequests,
		WebFetchRequests:    resp.Usage.ServerToolUse.WebFetchRequests,
		IsError:             resp.IsError,
		Errors:              resp.Errors,
	}, nil
}

// cleanLLMJSON extracts a JSON object from LLM output, handling markdown code
// fences and prose preambles that some models emit before the actual JSON.
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

	// If it already starts with '{' or '[', we're done.
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return s
	}

	// LLMs sometimes emit prose before the JSON object (e.g. "Based on my
	// analysis, here is the classification:"). Look for a markdown fence
	// embedded in the prose, or failing that, the first '{' or '['.

	// Check for an embedded ```json ... ``` block within the prose.
	if fenceStart := strings.Index(s, "```json"); fenceStart != -1 {
		inner := s[fenceStart+len("```json"):]
		if nl := strings.Index(inner, "\n"); nl != -1 {
			inner = inner[nl+1:]
		}
		if fenceEnd := strings.Index(inner, "```"); fenceEnd != -1 {
			inner = inner[:fenceEnd]
		}
		inner = strings.TrimSpace(inner)
		if strings.HasPrefix(inner, "{") || strings.HasPrefix(inner, "[") {
			return inner
		}
	}
	if fenceStart := strings.Index(s, "```\n"); fenceStart != -1 {
		inner := s[fenceStart+len("```\n"):]
		if fenceEnd := strings.Index(inner, "```"); fenceEnd != -1 {
			inner = inner[:fenceEnd]
		}
		inner = strings.TrimSpace(inner)
		if strings.HasPrefix(inner, "{") || strings.HasPrefix(inner, "[") {
			return inner
		}
	}

	// Last resort: find the first JSON value start: '{' (object) or '[' (array).
	braceStart := strings.IndexAny(s, "{[")
	if braceStart == -1 {
		return s // no JSON found; return as-is for the caller to report
	}
	openChar := s[braceStart]
	closeChar := byte('}')
	if openChar == '[' {
		closeChar = ']'
	}
	braceEnd := strings.LastIndexByte(s, closeChar)
	if braceEnd == -1 || braceEnd < braceStart {
		return s
	}
	return s[braceStart : braceEnd+1]
}

// isRetriableJSONError returns true if the error message indicates a JSON parse
// failure from LLM output — these are non-deterministic and worth retrying.
func isRetriableJSONError(errMsg string) bool {
	return strings.Contains(errMsg, "invalid JSON:")
}

func truncateForLog(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

// cleanClaudeEnv returns os.Environ() with CLAUDECODE stripped and CABRERO_SESSION=1 added.
func cleanClaudeEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		env = append(env, e)
	}
	return append(env, "CABRERO_SESSION=1")
}

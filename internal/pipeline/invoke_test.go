package pipeline

import (
	"regexp"
	"strings"
	"testing"
)

// --- buildClaudeArgs tests ---

func TestBuildClaudeArgs_AgenticBaseline(t *testing.T) {
	cfg := claudeConfig{
		Model:        "claude-haiku-4-5",
		SystemPrompt: "You are a classifier.",
		Agentic:      true,
		Prompt:       "classify this",
	}
	args := buildClaudeArgs(cfg, "")

	assertContains(t, args, "--model", "claude-haiku-4-5")
	assertContains(t, args, "-p", "classify this")
	assertContains(t, args, "--system-prompt", "You are a classifier.")
	assertContains(t, args, "--output-format", "text")
	assertHasFlag(t, args, "--disable-slash-commands")
	assertHasFlag(t, args, "--no-session-persistence")

	// Should NOT have print-mode flags.
	assertNotHasFlag(t, args, "--print")
	assertNotHasFlag(t, args, "--tools")
}

func TestBuildClaudeArgs_PrintBaseline(t *testing.T) {
	cfg := claudeConfig{
		Model:        "claude-haiku-4-5",
		SystemPrompt: "You are a parser.",
	}
	args := buildClaudeArgs(cfg, "")

	assertContains(t, args, "--model", "claude-haiku-4-5")
	assertHasFlag(t, args, "--print")
	assertContains(t, args, "--system-prompt", "You are a parser.")
	assertHasFlag(t, args, "--disable-slash-commands")
	assertContains(t, args, "--tools", "")
	assertHasFlag(t, args, "--no-session-persistence")

	// Should NOT have agentic-mode flags.
	assertNotHasFlag(t, args, "-p")
	assertNotHasFlag(t, args, "--output-format")
}

func TestBuildClaudeArgs_DebugMode(t *testing.T) {
	cfg := claudeConfig{
		Model:        "claude-haiku-4-5",
		SystemPrompt: "test",
		Agentic:      true,
		Prompt:       "test",
		Debug:        true,
	}
	args := buildClaudeArgs(cfg, "debug-uuid-123")

	assertContains(t, args, "--session-id", "debug-uuid-123")
	assertNotHasFlag(t, args, "--no-session-persistence")
}

func TestBuildClaudeArgs_DebugModePrint(t *testing.T) {
	cfg := claudeConfig{
		Model:        "claude-haiku-4-5",
		SystemPrompt: "test",
		Debug:        true,
	}
	args := buildClaudeArgs(cfg, "debug-uuid-456")

	assertContains(t, args, "--session-id", "debug-uuid-456")
	assertNotHasFlag(t, args, "--no-session-persistence")
}

func TestBuildClaudeArgs_AllowedTools(t *testing.T) {
	cfg := claudeConfig{
		Model:        "claude-haiku-4-5",
		SystemPrompt: "test",
		Agentic:      true,
		Prompt:       "test",
		AllowedTools: "Read(//home/**),Grep(//home/**)",
	}
	args := buildClaudeArgs(cfg, "")

	assertContains(t, args, "--allowedTools", "Read(//home/**),Grep(//home/**)")
}

func TestBuildClaudeArgs_AllowedToolsOmittedWhenEmpty(t *testing.T) {
	cfg := claudeConfig{
		Model:        "claude-haiku-4-5",
		SystemPrompt: "test",
		Agentic:      true,
		Prompt:       "test",
	}
	args := buildClaudeArgs(cfg, "")

	assertNotHasFlag(t, args, "--allowedTools")
}

func TestBuildClaudeArgs_Effort(t *testing.T) {
	t.Run("agentic", func(t *testing.T) {
		cfg := claudeConfig{
			Model:        "claude-sonnet-4-6",
			SystemPrompt: "test",
			Agentic:      true,
			Prompt:       "test",
			Effort:       "high",
		}
		args := buildClaudeArgs(cfg, "")
		assertContains(t, args, "--effort", "high")
	})

	t.Run("print", func(t *testing.T) {
		cfg := claudeConfig{
			Model:        "claude-sonnet-4-6",
			SystemPrompt: "test",
			Effort:       "high",
		}
		args := buildClaudeArgs(cfg, "")
		assertContains(t, args, "--effort", "high")
	})
}

func TestBuildClaudeArgs_DisallowedTools(t *testing.T) {
	cfg := claudeConfig{
		Model:           "claude-haiku-4-5",
		SystemPrompt:    "test",
		Agentic:         true,
		Prompt:          "test",
		DisallowedTools: "Write,Edit",
	}
	args := buildClaudeArgs(cfg, "")

	assertContains(t, args, "--disallowedTools", "Write,Edit")
}

func TestBuildClaudeArgs_DisallowedToolsOnlyAgentic(t *testing.T) {
	// DisallowedTools is only appended in agentic mode.
	cfg := claudeConfig{
		Model:           "claude-haiku-4-5",
		SystemPrompt:    "test",
		DisallowedTools: "Write",
	}
	args := buildClaudeArgs(cfg, "")

	assertNotHasFlag(t, args, "--disallowedTools")
}

func TestBuildClaudeArgs_PermissionMode(t *testing.T) {
	t.Run("agentic", func(t *testing.T) {
		cfg := claudeConfig{
			Model:          "claude-haiku-4-5",
			SystemPrompt:   "test",
			Agentic:        true,
			Prompt:         "test",
			PermissionMode: "dontAsk",
		}
		args := buildClaudeArgs(cfg, "")
		assertContains(t, args, "--permission-mode", "dontAsk")
	})

	t.Run("print", func(t *testing.T) {
		cfg := claudeConfig{
			Model:          "claude-haiku-4-5",
			SystemPrompt:   "test",
			PermissionMode: "dontAsk",
		}
		args := buildClaudeArgs(cfg, "")
		assertContains(t, args, "--permission-mode", "dontAsk")
	})
}

func TestBuildClaudeArgs_SettingSources(t *testing.T) {
	t.Run("nil omits flag", func(t *testing.T) {
		cfg := claudeConfig{
			Model:        "claude-haiku-4-5",
			SystemPrompt: "test",
			Agentic:      true,
			Prompt:       "test",
		}
		args := buildClaudeArgs(cfg, "")
		assertNotHasFlag(t, args, "--setting-sources")
	})

	t.Run("empty string passes empty value", func(t *testing.T) {
		empty := ""
		cfg := claudeConfig{
			Model:          "claude-haiku-4-5",
			SystemPrompt:   "test",
			Agentic:        true,
			Prompt:         "test",
			SettingSources: &empty,
		}
		args := buildClaudeArgs(cfg, "")
		assertContains(t, args, "--setting-sources", "")
	})

	t.Run("non-empty value passed through", func(t *testing.T) {
		val := "project"
		cfg := claudeConfig{
			Model:          "claude-haiku-4-5",
			SystemPrompt:   "test",
			Agentic:        true,
			Prompt:         "test",
			SettingSources: &val,
		}
		args := buildClaudeArgs(cfg, "")
		assertContains(t, args, "--setting-sources", "project")
	})

	t.Run("print mode passes flag", func(t *testing.T) {
		empty := ""
		cfg := claudeConfig{
			Model:          "claude-haiku-4-5",
			SystemPrompt:   "test",
			SettingSources: &empty,
		}
		args := buildClaudeArgs(cfg, "")
		assertContains(t, args, "--setting-sources", "")
	})
}

func TestBuildClaudeArgs_FullIsolation(t *testing.T) {
	// Simulates the classifier's full isolation config.
	empty := ""
	cfg := claudeConfig{
		Model:          "claude-haiku-4-5",
		SystemPrompt:   "You are a classifier.",
		Agentic:        true,
		Prompt:         "<session_digest>...</session_digest>",
		AllowedTools:   "Read(//home/.cabrero/**),Grep(//home/.cabrero/**)",
		PermissionMode: "dontAsk",
		SettingSources: &empty,
	}
	args := buildClaudeArgs(cfg, "")

	assertContains(t, args, "--allowedTools", "Read(//home/.cabrero/**),Grep(//home/.cabrero/**)")
	assertContains(t, args, "--permission-mode", "dontAsk")
	assertContains(t, args, "--setting-sources", "")
	assertHasFlag(t, args, "--no-session-persistence")
}

// --- evaluatorAllowedTools tests ---

func TestEvaluatorAllowedTools_NilCwd(t *testing.T) {
	result := evaluatorAllowedTools(nil)

	// Should have cabrero and claude paths, not a project path.
	assertToolPathPresent(t, result, "Read", ".cabrero")
	assertToolPathPresent(t, result, "Grep", ".cabrero")
	assertToolPathPresent(t, result, "Read", ".claude")
	assertToolPathPresent(t, result, "Grep", ".claude")

	// Should have exactly 4 entries (2 cabrero + 2 claude).
	parts := strings.Split(result, ",")
	if len(parts) != 4 {
		t.Errorf("expected 4 tool entries, got %d: %v", len(parts), parts)
	}
}

func TestEvaluatorAllowedTools_EmptyCwd(t *testing.T) {
	empty := ""
	result := evaluatorAllowedTools(&empty)

	// Empty cwd should be skipped — same as nil.
	parts := strings.Split(result, ",")
	if len(parts) != 4 {
		t.Errorf("expected 4 tool entries (empty cwd skipped), got %d: %v", len(parts), parts)
	}
}

func TestEvaluatorAllowedTools_WithCwd(t *testing.T) {
	cwd := "/Users/test/projects/myapp"
	result := evaluatorAllowedTools(&cwd)

	// Should have cabrero + project + claude paths.
	assertToolPathPresent(t, result, "Read", ".cabrero")
	assertToolPathPresent(t, result, "Grep", ".cabrero")
	assertToolPathPresent(t, result, "Read", "/Users/test/projects/myapp")
	assertToolPathPresent(t, result, "Grep", "/Users/test/projects/myapp")
	assertToolPathPresent(t, result, "Read", ".claude")
	assertToolPathPresent(t, result, "Grep", ".claude")

	// Should have exactly 6 entries.
	parts := strings.Split(result, ",")
	if len(parts) != 6 {
		t.Errorf("expected 6 tool entries, got %d: %v", len(parts), parts)
	}
}

func TestEvaluatorAllowedTools_PathFormat(t *testing.T) {
	cwd := "/Users/test/project"
	result := evaluatorAllowedTools(&cwd)

	// Every entry must use // prefix and /** suffix.
	for _, part := range strings.Split(result, ",") {
		if !strings.Contains(part, "//") {
			t.Errorf("entry missing // prefix: %s", part)
		}
		if !strings.HasSuffix(part, "/**)") {
			t.Errorf("entry missing /** suffix: %s", part)
		}
	}
}

// --- generateUUID tests ---

func TestGenerateUUID_Format(t *testing.T) {
	uuid, err := generateUUID()
	if err != nil {
		t.Fatalf("generateUUID: %v", err)
	}

	// UUID v4 format: 8-4-4-4-12 hex chars.
	pattern := `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
	matched, err := regexp.MatchString(pattern, uuid)
	if err != nil {
		t.Fatalf("regexp: %v", err)
	}
	if !matched {
		t.Errorf("UUID %q does not match 8-4-4-4-12 format", uuid)
	}
}

func TestGenerateUUID_Version4(t *testing.T) {
	uuid, err := generateUUID()
	if err != nil {
		t.Fatalf("generateUUID: %v", err)
	}

	// The 13th character (index 14, after 2 dashes) is the version nibble.
	// In format 8-4-4-4-12, position 14 is the first char of the 3rd group.
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts, got %d", len(parts))
	}
	// Third group starts with version nibble.
	if parts[2][0] != '4' {
		t.Errorf("version nibble = %c, want '4' (UUID: %s)", parts[2][0], uuid)
	}
}

func TestGenerateUUID_VariantRFC4122(t *testing.T) {
	uuid, err := generateUUID()
	if err != nil {
		t.Fatalf("generateUUID: %v", err)
	}

	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts, got %d", len(parts))
	}
	// Fourth group first char is the variant nibble — must be 8, 9, a, or b.
	variant := parts[3][0]
	if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
		t.Errorf("variant nibble = %c, want 8/9/a/b (UUID: %s)", variant, uuid)
	}
}

func TestGenerateUUID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		uuid, err := generateUUID()
		if err != nil {
			t.Fatalf("generateUUID iteration %d: %v", i, err)
		}
		if seen[uuid] {
			t.Fatalf("duplicate UUID at iteration %d: %s", i, uuid)
		}
		seen[uuid] = true
	}
}

// --- Test helpers ---

// assertContains checks that args contains the given flag followed by value.
func assertContains(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == value {
			return
		}
	}
	t.Errorf("args missing %s %q: %v", flag, value, args)
}

// assertHasFlag checks that args contains the given flag (no value check).
func assertHasFlag(t *testing.T, args []string, flag string) {
	t.Helper()
	for _, a := range args {
		if a == flag {
			return
		}
	}
	t.Errorf("args missing flag %s: %v", flag, args)
}

// assertNotHasFlag checks that args does NOT contain the given flag.
func assertNotHasFlag(t *testing.T, args []string, flag string) {
	t.Helper()
	for _, a := range args {
		if a == flag {
			t.Errorf("args unexpectedly contains %s: %v", flag, args)
			return
		}
	}
}

// assertToolPathPresent checks that the result string contains a Tool(//...path../**) entry.
func assertToolPathPresent(t *testing.T, result, tool, pathSubstr string) {
	t.Helper()
	// Look for e.g. "Read(//...pathSubstr...**)"
	pattern := tool + "(//"
	for _, part := range strings.Split(result, ",") {
		if strings.HasPrefix(part, pattern) && strings.Contains(part, pathSubstr) {
			return
		}
	}
	t.Errorf("missing %s entry containing %q in: %s", tool, pathSubstr, result)
}

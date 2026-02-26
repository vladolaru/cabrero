package pipeline

import (
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
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
	assertContains(t, args, "--output-format", "json")
	assertHasFlag(t, args, "--disable-slash-commands")
	// Agentic mode never uses --no-session-persistence; session is always persisted.
	assertNotHasFlag(t, args, "--no-session-persistence")

	// Should NOT have print-mode flags.
	assertNotHasFlag(t, args, "--print")
	assertNotHasFlag(t, args, "--tools")
}

func TestBuildClaudeArgs_AgenticAlwaysHasSessionID(t *testing.T) {
	// With Debug=false, agentic mode must still generate --session-id
	// and must NOT include --no-session-persistence.
	cfg := claudeConfig{
		Model:   "claude-haiku-4-5",
		Agentic: true,
		Prompt:  "test prompt",
		Debug:   false,
	}
	args := buildClaudeArgs(cfg, "some-uuid")
	hasSessionID := false
	hasPersistFlag := false
	for i, a := range args {
		if a == "--session-id" && i+1 < len(args) {
			hasSessionID = true
		}
		if a == "--no-session-persistence" {
			hasPersistFlag = true
		}
	}
	if !hasSessionID {
		t.Error("agentic mode must include --session-id even when Debug=false")
	}
	if hasPersistFlag {
		t.Error("agentic mode must not include --no-session-persistence")
	}
}

func TestBuildClaudeArgs_PrintModeKeepsNoPersistence(t *testing.T) {
	cfg := claudeConfig{
		Model:   "claude-sonnet-4-6",
		Agentic: false,
		Debug:   false,
	}
	args := buildClaudeArgs(cfg, "")
	hasNoPersist := false
	for _, a := range args {
		if a == "--no-session-persistence" {
			hasNoPersist = true
		}
	}
	if !hasNoPersist {
		t.Error("print mode must keep --no-session-persistence")
	}
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
	// Agentic mode never uses --no-session-persistence.
	assertNotHasFlag(t, args, "--no-session-persistence")
}

// --- cleanLLMJSON tests ---

func TestCleanLLMJSON_PlainJSON(t *testing.T) {
	input := `{"triage": "evaluate"}`
	got := cleanLLMJSON(input)
	if got != input {
		t.Errorf("cleanLLMJSON(%q) = %q, want %q", input, got, input)
	}
}

func TestCleanLLMJSON_MarkdownFence(t *testing.T) {
	input := "```json\n{\"triage\": \"evaluate\"}\n```"
	want := `{"triage": "evaluate"}`
	got := cleanLLMJSON(input)
	if got != want {
		t.Errorf("cleanLLMJSON = %q, want %q", got, want)
	}
}

func TestCleanLLMJSON_ProseBeforeJSON(t *testing.T) {
	input := "Based on my analysis of the session, here is my classification:\n\n{\"triage\": \"evaluate\", \"version\": 2}"
	want := `{"triage": "evaluate", "version": 2}`
	got := cleanLLMJSON(input)
	if got != want {
		t.Errorf("cleanLLMJSON = %q, want %q", got, want)
	}
}

func TestCleanLLMJSON_ProseWithEmbeddedFence(t *testing.T) {
	input := "I have analyzed the session. Here is my output:\n\n```json\n{\"triage\": \"clean\"}\n```\n"
	want := `{"triage": "clean"}`
	got := cleanLLMJSON(input)
	if got != want {
		t.Errorf("cleanLLMJSON = %q, want %q", got, want)
	}
}

func TestCleanLLMJSON_ProseWithEmbeddedBareFence(t *testing.T) {
	input := "Here is the result:\n\n```\n{\"version\": 2}\n```"
	want := `{"version": 2}`
	got := cleanLLMJSON(input)
	if got != want {
		t.Errorf("cleanLLMJSON = %q, want %q", got, want)
	}
}

func TestCleanLLMJSON_WhitespaceAroundJSON(t *testing.T) {
	input := "  \n  {\"triage\": \"evaluate\"}  \n  "
	want := `{"triage": "evaluate"}`
	got := cleanLLMJSON(input)
	if got != want {
		t.Errorf("cleanLLMJSON = %q, want %q", got, want)
	}
}

func TestCleanLLMJSON_ProseAfterJSON(t *testing.T) {
	input := "{\"triage\": \"clean\"}\n\nThis completes the analysis."
	// The last '}' is the one in the JSON, so brace matching should work.
	got := cleanLLMJSON(input)
	if !strings.HasPrefix(got, "{") {
		t.Errorf("cleanLLMJSON should start with '{', got %q", got)
	}
	if !strings.Contains(got, `"triage"`) {
		t.Errorf("cleanLLMJSON should contain triage field, got %q", got)
	}
}

func TestCleanLLMJSON_NoBraces(t *testing.T) {
	input := "No JSON here at all"
	got := cleanLLMJSON(input)
	// Should return as-is when no JSON found.
	if got != input {
		t.Errorf("cleanLLMJSON = %q, want %q (passthrough)", got, input)
	}
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

// --- GenerateUUID tests ---

func TestGenerateUUID_Format(t *testing.T) {
	uuid, err := GenerateUUID()
	if err != nil {
		t.Fatalf("GenerateUUID: %v", err)
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
	uuid, err := GenerateUUID()
	if err != nil {
		t.Fatalf("GenerateUUID: %v", err)
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
	uuid, err := GenerateUUID()
	if err != nil {
		t.Fatalf("GenerateUUID: %v", err)
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
		uuid, err := GenerateUUID()
		if err != nil {
			t.Fatalf("GenerateUUID iteration %d: %v", i, err)
		}
		if seen[uuid] {
			t.Fatalf("duplicate UUID at iteration %d: %s", i, uuid)
		}
		seen[uuid] = true
	}
}

// --- parseClaudeJSON tests ---

func TestParseClaudeJSON_Success(t *testing.T) {
	jsonData := `{
		"type": "result",
		"subtype": "success",
		"is_error": false,
		"result": "{\"triage\": \"evaluate\"}",
		"session_id": "abc-123-def",
		"num_turns": 5,
		"duration_ms": 12345,
		"duration_api_ms": 9000,
		"total_cost_usd": 0.0234,
		"usage": {
			"input_tokens": 5000,
			"output_tokens": 1500,
			"cache_creation_input_tokens": 200,
			"cache_read_input_tokens": 3000,
			"server_tool_use": {
				"web_search_requests": 0,
				"web_fetch_requests": 0
			}
		}
	}`

	cr, err := parseClaudeJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("parseClaudeJSON: %v", err)
	}

	if cr.Result != `{"triage": "evaluate"}` {
		t.Errorf("Result = %q, want JSON string", cr.Result)
	}
	if cr.SessionID != "abc-123-def" {
		t.Errorf("SessionID = %q, want %q", cr.SessionID, "abc-123-def")
	}
	if cr.NumTurns != 5 {
		t.Errorf("NumTurns = %d, want 5", cr.NumTurns)
	}
	if cr.DurationMs != 12345 {
		t.Errorf("DurationMs = %d, want 12345", cr.DurationMs)
	}
	if cr.DurationApiMs != 9000 {
		t.Errorf("DurationApiMs = %d, want 9000", cr.DurationApiMs)
	}
	if cr.TotalCostUSD != 0.0234 {
		t.Errorf("TotalCostUSD = %f, want 0.0234", cr.TotalCostUSD)
	}
	if cr.InputTokens != 5000 {
		t.Errorf("InputTokens = %d, want 5000", cr.InputTokens)
	}
	if cr.OutputTokens != 1500 {
		t.Errorf("OutputTokens = %d, want 1500", cr.OutputTokens)
	}
	if cr.CacheCreationTokens != 200 {
		t.Errorf("CacheCreationTokens = %d, want 200", cr.CacheCreationTokens)
	}
	if cr.CacheReadTokens != 3000 {
		t.Errorf("CacheReadTokens = %d, want 3000", cr.CacheReadTokens)
	}
	if cr.IsError {
		t.Error("IsError = true, want false")
	}
	if len(cr.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", cr.Errors)
	}
}

func TestParseClaudeJSON_Error(t *testing.T) {
	jsonData := `{
		"type": "result",
		"subtype": "error_response",
		"is_error": true,
		"result": "Something went wrong",
		"session_id": "err-session-1",
		"num_turns": 2,
		"duration_ms": 5000,
		"duration_api_ms": 3000,
		"total_cost_usd": 0.005,
		"usage": {
			"input_tokens": 1000,
			"output_tokens": 200,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens": 0,
			"server_tool_use": {
				"web_search_requests": 0,
				"web_fetch_requests": 0
			}
		},
		"errors": ["rate limit exceeded", "retry later"]
	}`

	cr, err := parseClaudeJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("parseClaudeJSON: %v", err)
	}

	if !cr.IsError {
		t.Error("IsError = false, want true")
	}
	if len(cr.Errors) != 2 {
		t.Fatalf("Errors len = %d, want 2", len(cr.Errors))
	}
	if cr.Errors[0] != "rate limit exceeded" {
		t.Errorf("Errors[0] = %q, want %q", cr.Errors[0], "rate limit exceeded")
	}
	// Usage should still be captured even on error.
	if cr.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", cr.InputTokens)
	}
	if cr.OutputTokens != 200 {
		t.Errorf("OutputTokens = %d, want 200", cr.OutputTokens)
	}
	if cr.TotalCostUSD != 0.005 {
		t.Errorf("TotalCostUSD = %f, want 0.005", cr.TotalCostUSD)
	}
}

func TestParseClaudeJSON_Malformed(t *testing.T) {
	_, err := parseClaudeJSON([]byte("not json at all"))
	if err == nil {
		t.Fatal("expected error for malformed input")
	}
	if !strings.Contains(err.Error(), "invalid claude JSON") {
		t.Errorf("error = %q, want to contain 'invalid claude JSON'", err.Error())
	}
}

func TestParseClaudeJSON_EmptyResult(t *testing.T) {
	jsonData := `{
		"type": "result",
		"is_error": false,
		"result": "",
		"session_id": "empty-result",
		"num_turns": 1,
		"duration_ms": 100,
		"duration_api_ms": 50,
		"total_cost_usd": 0.001,
		"usage": {
			"input_tokens": 100,
			"output_tokens": 10,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens": 0,
			"server_tool_use": {
				"web_search_requests": 0,
				"web_fetch_requests": 0
			}
		}
	}`

	cr, err := parseClaudeJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("parseClaudeJSON: %v", err)
	}

	if cr.Result != "" {
		t.Errorf("Result = %q, want empty string", cr.Result)
	}
	if cr.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", cr.InputTokens)
	}
}

func TestParseClaudeJSON_WebSearchUsage(t *testing.T) {
	jsonData := `{
		"type": "result",
		"is_error": false,
		"result": "search results",
		"session_id": "web-search-1",
		"num_turns": 3,
		"duration_ms": 8000,
		"duration_api_ms": 6000,
		"total_cost_usd": 0.015,
		"usage": {
			"input_tokens": 3000,
			"output_tokens": 800,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens": 0,
			"server_tool_use": {
				"web_search_requests": 2,
				"web_fetch_requests": 3
			}
		}
	}`

	cr, err := parseClaudeJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("parseClaudeJSON: %v", err)
	}

	if cr.WebSearchRequests != 2 {
		t.Errorf("WebSearchRequests = %d, want 2", cr.WebSearchRequests)
	}
	if cr.WebFetchRequests != 3 {
		t.Errorf("WebFetchRequests = %d, want 3", cr.WebFetchRequests)
	}
}

// --- usageFromResult tests ---

func TestUsageFromResult_Nil(t *testing.T) {
	usage := usageFromResult(nil)
	if usage != nil {
		t.Errorf("usageFromResult(nil) = %v, want nil", usage)
	}
}

func TestUsageFromResult_MapsFields(t *testing.T) {
	cr := &ClaudeResult{
		SessionID:           "sess-123",
		NumTurns:            5,
		TotalCostUSD:        0.025,
		InputTokens:         5000,
		OutputTokens:        1500,
		CacheCreationTokens: 200,
		CacheReadTokens:     3000,
		WebSearchRequests:   1,
		WebFetchRequests:    2,
	}

	usage := usageFromResult(cr)
	if usage == nil {
		t.Fatal("usageFromResult returned nil for non-nil input")
	}
	if usage.CCSessionID != "sess-123" {
		t.Errorf("CCSessionID = %q, want %q", usage.CCSessionID, "sess-123")
	}
	if usage.NumTurns != 5 {
		t.Errorf("NumTurns = %d, want 5", usage.NumTurns)
	}
	if usage.CostUSD != 0.025 {
		t.Errorf("CostUSD = %f, want 0.025", usage.CostUSD)
	}
	if usage.InputTokens != 5000 {
		t.Errorf("InputTokens = %d, want 5000", usage.InputTokens)
	}
	if usage.OutputTokens != 1500 {
		t.Errorf("OutputTokens = %d, want 1500", usage.OutputTokens)
	}
	if usage.CacheCreationTokens != 200 {
		t.Errorf("CacheCreationTokens = %d, want 200", usage.CacheCreationTokens)
	}
	if usage.CacheReadTokens != 3000 {
		t.Errorf("CacheReadTokens = %d, want 3000", usage.CacheReadTokens)
	}
	if usage.WebSearchRequests != 1 {
		t.Errorf("WebSearchRequests = %d, want 1", usage.WebSearchRequests)
	}
	if usage.WebFetchRequests != 2 {
		t.Errorf("WebFetchRequests = %d, want 2", usage.WebFetchRequests)
	}
}

// --- computeUsageTotals tests ---

func TestComputeUsageTotals_BothStages(t *testing.T) {
	rec := HistoryRecord{
		ClassifierUsage: &InvocationUsage{
			InputTokens:  5000,
			OutputTokens: 1500,
			CostUSD:      0.01,
		},
		EvaluatorUsage: &InvocationUsage{
			InputTokens:  10000,
			OutputTokens: 3000,
			CostUSD:      0.03,
		},
	}

	rec.computeUsageTotals()

	if rec.TotalInputTokens != 15000 {
		t.Errorf("TotalInputTokens = %d, want 15000", rec.TotalInputTokens)
	}
	if rec.TotalOutputTokens != 4500 {
		t.Errorf("TotalOutputTokens = %d, want 4500", rec.TotalOutputTokens)
	}
	if rec.TotalCostUSD != 0.04 {
		t.Errorf("TotalCostUSD = %f, want 0.04", rec.TotalCostUSD)
	}
}

func TestComputeUsageTotals_ClassifierOnly(t *testing.T) {
	rec := HistoryRecord{
		ClassifierUsage: &InvocationUsage{
			InputTokens:  5000,
			OutputTokens: 1500,
			CostUSD:      0.01,
		},
	}

	rec.computeUsageTotals()

	if rec.TotalInputTokens != 5000 {
		t.Errorf("TotalInputTokens = %d, want 5000", rec.TotalInputTokens)
	}
	if rec.TotalOutputTokens != 1500 {
		t.Errorf("TotalOutputTokens = %d, want 1500", rec.TotalOutputTokens)
	}
	if rec.TotalCostUSD != 0.01 {
		t.Errorf("TotalCostUSD = %f, want 0.01", rec.TotalCostUSD)
	}
}

func TestComputeUsageTotals_NoUsage(t *testing.T) {
	rec := HistoryRecord{}

	rec.computeUsageTotals()

	if rec.TotalInputTokens != 0 {
		t.Errorf("TotalInputTokens = %d, want 0", rec.TotalInputTokens)
	}
	if rec.TotalOutputTokens != 0 {
		t.Errorf("TotalOutputTokens = %d, want 0", rec.TotalOutputTokens)
	}
	if rec.TotalCostUSD != 0 {
		t.Errorf("TotalCostUSD = %f, want 0", rec.TotalCostUSD)
	}
}

// --- splitUsageForBatch tests ---

func TestSplitUsageForBatch_NilResult(t *testing.T) {
	result := splitUsageForBatch(nil, 3)
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}
	for i, u := range result {
		if u != nil {
			t.Errorf("result[%d] = %v, want nil", i, u)
		}
	}
}

func TestSplitUsageForBatch_SplitsEvenly(t *testing.T) {
	cr := &ClaudeResult{
		SessionID:           "batch-sess",
		NumTurns:            10,
		TotalCostUSD:        0.06,
		InputTokens:         9000,
		OutputTokens:        3000,
		CacheCreationTokens: 600,
		CacheReadTokens:     1200,
		WebSearchRequests:   2,
		WebFetchRequests:    4,
	}

	result := splitUsageForBatch(cr, 3)
	if len(result) != 3 {
		t.Fatalf("len = %d, want 3", len(result))
	}

	for i, u := range result {
		if u == nil {
			t.Fatalf("result[%d] is nil", i)
		}
		// All share the same session ID.
		if u.CCSessionID != "batch-sess" {
			t.Errorf("[%d] CCSessionID = %q, want %q", i, u.CCSessionID, "batch-sess")
		}
		// NumTurns is shared (not divisible).
		if u.NumTurns != 10 {
			t.Errorf("[%d] NumTurns = %d, want 10", i, u.NumTurns)
		}
		// Tokens split equally.
		if u.InputTokens != 3000 {
			t.Errorf("[%d] InputTokens = %d, want 3000", i, u.InputTokens)
		}
		if u.OutputTokens != 1000 {
			t.Errorf("[%d] OutputTokens = %d, want 1000", i, u.OutputTokens)
		}
		if u.CacheCreationTokens != 200 {
			t.Errorf("[%d] CacheCreationTokens = %d, want 200", i, u.CacheCreationTokens)
		}
		if u.CacheReadTokens != 400 {
			t.Errorf("[%d] CacheReadTokens = %d, want 400", i, u.CacheReadTokens)
		}
		// Cost split equally.
		if u.CostUSD != 0.02 {
			t.Errorf("[%d] CostUSD = %f, want 0.02", i, u.CostUSD)
		}
		// Web requests are shared (not divided).
		if u.WebSearchRequests != 2 {
			t.Errorf("[%d] WebSearchRequests = %d, want 2", i, u.WebSearchRequests)
		}
	}
}

func TestSplitUsageForBatch_SingleSession(t *testing.T) {
	cr := &ClaudeResult{
		SessionID:    "single-sess",
		InputTokens:  5000,
		OutputTokens: 1500,
		TotalCostUSD: 0.01,
	}

	result := splitUsageForBatch(cr, 1)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].InputTokens != 5000 {
		t.Errorf("InputTokens = %d, want 5000", result[0].InputTokens)
	}
	if result[0].OutputTokens != 1500 {
		t.Errorf("OutputTokens = %d, want 1500", result[0].OutputTokens)
	}
}

// --- invokeSemaphore tests ---

func TestInvokeSemaphore_LimitsConcurrency(t *testing.T) {
	resetInvokeSemaphore()

	limit := 2
	InitInvokeSemaphore(limit)

	var mu sync.Mutex
	maxConcurrent := 0
	current := 0
	done := make(chan struct{})

	for i := 0; i < 5; i++ {
		go func() {
			acquireInvokeSemaphore()
			mu.Lock()
			current++
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()
			releaseInvokeSemaphore()

			done <- struct{}{}
		}()
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	if maxConcurrent > limit {
		t.Errorf("maxConcurrent = %d, want <= %d", maxConcurrent, limit)
	}
	if maxConcurrent < limit {
		t.Errorf("maxConcurrent = %d, want exactly %d (semaphore not fully utilized)", maxConcurrent, limit)
	}
}

func TestInvokeSemaphore_ZeroMeansUnlimited(t *testing.T) {
	resetInvokeSemaphore()
	InitInvokeSemaphore(0)

	// Should not block — acquire/release are no-ops.
	acquireInvokeSemaphore()
	releaseInvokeSemaphore()
}

func TestTryAcquireInvokeSemaphore_Succeeds(t *testing.T) {
	resetInvokeSemaphore()
	InitInvokeSemaphore(1)

	if !TryAcquireInvokeSemaphore() {
		t.Fatal("TryAcquire returned false on empty semaphore")
	}
	ReleaseInvokeSemaphore()
}

func TestTryAcquireInvokeSemaphore_FailsWhenFull(t *testing.T) {
	resetInvokeSemaphore()
	InitInvokeSemaphore(1)

	// Fill the single slot.
	if !TryAcquireInvokeSemaphore() {
		t.Fatal("first TryAcquire failed")
	}

	// Second attempt should fail immediately.
	if TryAcquireInvokeSemaphore() {
		t.Fatal("TryAcquire returned true on full semaphore")
	}

	ReleaseInvokeSemaphore()

	// After release, should succeed again.
	if !TryAcquireInvokeSemaphore() {
		t.Fatal("TryAcquire failed after release")
	}
	ReleaseInvokeSemaphore()
}

func TestTryAcquireInvokeSemaphore_UnlimitedAlwaysSucceeds(t *testing.T) {
	resetInvokeSemaphore()
	InitInvokeSemaphore(0)

	for i := 0; i < 10; i++ {
		if !TryAcquireInvokeSemaphore() {
			t.Fatalf("TryAcquire returned false with unlimited semaphore (iteration %d)", i)
		}
	}
	// No release needed — unlimited mode is a no-op.
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

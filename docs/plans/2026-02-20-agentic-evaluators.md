# Agentic Evaluators Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Convert the Haiku classifier and Sonnet evaluator from single-shot `--print` invocations to agentic tool-using sessions with Read/Grep access.

**Architecture:** Both LLM stages switch from piped stdin (`--print`) to `-p` prompt mode with `--allowedTools Read,Grep`. Haiku gets scoped access to `~/.cabrero/raw/` (via prompt instruction), Sonnet gets unrestricted filesystem read access. A new triage field in Haiku's output lets the pipeline skip Sonnet for clean sessions. Max turns and timeouts are configurable via daemon config. The daemon batches Sonnet invocations per-project when multiple sessions are pending — individual Haiku triage, then one Sonnet call per project batch.

**Tech Stack:** Go, `claude` CLI (`-p`, `--allowedTools`, `--max-turns`)

---

## Verification prerequisite

Before starting implementation, verify `claude` CLI supports these flags:

```bash
claude --help | grep -E "(allowedTools|max-turns|output-format)"
claude -p "Say hello" --allowedTools Read --max-turns 2 --model claude-haiku-4-5
```

If flag names differ, adjust all tasks accordingly. The `--max-turns` flag
is critical — without it, guardrails depend solely on timeout.

For timeout, verify whether `claude` has a `--timeout` flag. If not, use
`exec.CommandContext` with `context.WithTimeout`.

---

### Task 1: Add PipelineConfig struct

**Files:**
- Modify: `internal/pipeline/pipeline.go:11-22` (add config, update Run signature)
- Modify: `internal/daemon/daemon.go:17-34` (add pipeline fields to Config)
- Modify: `internal/daemon/daemon.go:126-149` (pass config to pipeline.Run)
- Modify: `internal/cmd/daemon.go:16-48` (thread config)
- Modify: `internal/cmd/run.go` (pass default config)

**Step 1: Add PipelineConfig to pipeline.go**

Add above `RunResult`:

```go
// PipelineConfig controls LLM invocation parameters.
type PipelineConfig struct {
	HaikuMaxTurns  int
	SonnetMaxTurns int
	HaikuTimeout   time.Duration
	SonnetTimeout  time.Duration
}

// DefaultPipelineConfig returns production defaults.
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		HaikuMaxTurns:  15,
		SonnetMaxTurns: 20,
		HaikuTimeout:   2 * time.Minute,
		SonnetTimeout:  5 * time.Minute,
	}
}
```

**Step 2: Update Run signature**

```go
func Run(sessionID string, dryRun bool, cfg PipelineConfig) (*RunResult, error) {
```

Pass `cfg` to `RunHaiku` and `RunSonnet` (update signatures but don't use
the config yet — agentic mode isn't wired up until Task 4/5).

```go
func RunHaiku(sessionID string, digest *parser.Digest, aggregatorOutput *patterns.AggregatorOutput, cfg PipelineConfig) (*HaikuOutput, error) {
```

```go
func RunSonnet(sessionID string, digest *parser.Digest, haikuOutput *HaikuOutput, cfg PipelineConfig) (*SonnetOutput, error) {
```

**Step 3: Add pipeline config fields to daemon.Config**

```go
type Config struct {
	PollInterval      time.Duration
	StaleInterval     time.Duration
	InterSessionDelay time.Duration
	LogPath           string
	LogMaxSize        int64
	Pipeline          pipeline.PipelineConfig
}
```

Update `DefaultConfig()` to set `Pipeline: pipeline.DefaultPipelineConfig()`.

**Step 4: Thread config through daemon → pipeline**

In `daemon.go` `processOne()`:

```go
result, err := pipeline.Run(sessionID, false, d.config.Pipeline)
```

In `cmd/run.go`:

```go
result, err := pipeline.Run(sessionID, *dryRun, pipeline.DefaultPipelineConfig())
```

**Step 5: Build and verify**

```bash
make build
./cabrero help
```

**Step 6: Commit**

```
refactor(pipeline): add PipelineConfig for agentic evaluator parameters

Thread configurable max turns and timeouts through daemon → pipeline →
LLM stages. No behavior change yet — config is accepted but not used
until Haiku and Sonnet switch to agentic invocation.
```

---

### Task 2: Refactor invokeClaude for dual mode

**Files:**
- Modify: `internal/pipeline/invoke.go:14-51` (extend claudeConfig, dual mode)

**Step 1: Extend claudeConfig**

```go
type claudeConfig struct {
	Model        string
	SystemPrompt string
	Effort       string
	// Agentic mode fields (ignored when Agentic is false).
	Agentic      bool
	Prompt       string        // user prompt via -p (agentic mode)
	AllowedTools string        // comma-separated tool names
	MaxTurns     int           // --max-turns limit
	Timeout      time.Duration // hard wall-clock timeout
}
```

**Step 2: Update invokeClaude to support both modes**

```go
func invokeClaude(cfg claudeConfig) (string, error) {
	var args []string

	if cfg.Agentic {
		args = []string{
			"--model", cfg.Model,
			"-p", cfg.Prompt,
			"--system-prompt", cfg.SystemPrompt,
			"--no-session-persistence",
			"--disable-slash-commands",
		}
		if cfg.AllowedTools != "" {
			args = append(args, "--allowedTools", cfg.AllowedTools)
		}
		if cfg.MaxTurns > 0 {
			args = append(args, "--max-turns", strconv.Itoa(cfg.MaxTurns))
		}
		if cfg.Effort != "" {
			args = append(args, "--effort", cfg.Effort)
		}
	} else {
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

	var cmd *exec.Cmd
	if cfg.Timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, "claude", args...)
	} else {
		cmd = exec.Command("claude", args...)
	}

	cmd.Env = append(os.Environ(), "CABRERO_SESSION=1")

	out, err := cmd.Output()
	if err != nil {
		if ctx_err := cmd.ProcessState; cfg.Timeout > 0 && ctx_err != nil {
			// Check if it was a timeout
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude exited with code %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("running claude: %w", err)
	}

	return string(out), nil
}
```

Note: remove the `stdin io.Reader` parameter. It's no longer needed because:
- `--print` mode: data now goes into `cfg.Prompt` (will be piped via stdin
  only if we keep backward compat — see step 3)
- Agentic mode: data goes via `-p` flag

**Step 3: Update existing callers**

For now, keep `--print` mode working by adding stdin back as an optional
field or by putting the current stdin data into the Prompt field for
`--print` mode. The simplest approach: add a `Stdin io.Reader` field to
`claudeConfig` that's only used when `Agentic` is false:

```go
type claudeConfig struct {
	// ... existing fields ...
	Stdin io.Reader // only used when Agentic is false (--print mode)
}
```

Update the `--print` branch to pipe `cfg.Stdin`:

```go
cmd.Stdin = cfg.Stdin
```

Update `RunHaiku` and `RunSonnet` callers to pass `Stdin`:

```go
invokeClaude(claudeConfig{
	Model:        "claude-haiku-4-5",
	SystemPrompt: systemPrompt,
	Stdin:        strings.NewReader(data),
})
```

**Step 4: Add required imports**

Add `context`, `strconv` to invoke.go imports.

**Step 5: Build and verify**

```bash
make build
```

**Step 6: Commit**

```
refactor(pipeline): support agentic invocation mode in invokeClaude

Add Agentic, Prompt, AllowedTools, MaxTurns, and Timeout fields to
claudeConfig. When Agentic is true, uses -p with --allowedTools and
--max-turns instead of --print with stdin. Existing callers unchanged.
```

---

### Task 3: Add triage field to HaikuOutput

**Files:**
- Modify: `internal/pipeline/proposal.go:16-27` (add Triage field)
- Modify: `internal/pipeline/pipeline.go:88-98` (triage gate)

**Step 1: Add Triage to HaikuOutput**

```go
type HaikuOutput struct {
	Version        int    `json:"version"`
	SessionID      string `json:"sessionId"`
	PromptVersion  string `json:"promptVersion"`
	Triage         string `json:"triage"` // "evaluate" or "clean"

	Goal                HaikuGoal                `json:"goal"`
	// ... rest unchanged
}
```

**Step 2: Add triage gate in pipeline.go**

After writing Haiku output (after line 86), add:

```go
// Triage gate: skip Sonnet if Haiku classified session as clean.
if haikuOutput.Triage == "clean" {
	fmt.Println("  Haiku triage: clean session — skipping Sonnet evaluator")
	if metaErr == nil {
		meta.Status = "processed"
		if err := store.WriteMetadata(store.RawDir(sessionID), meta); err != nil {
			fmt.Printf("  Warning: failed to update session status: %v\n", err)
		}
	}
	return result, nil
}
fmt.Println("  Haiku triage: session worth evaluating")
```

**Step 3: Default triage for backward compatibility**

In `RunHaiku()`, after parsing output, default empty triage to "evaluate":

```go
if output.Triage == "" {
	output.Triage = "evaluate"
}
```

This ensures existing v2 prompt output (which doesn't include triage)
still flows to Sonnet.

**Step 4: Build and verify**

```bash
make build
```

**Step 5: Commit**

```
feat(pipeline): add triage gate between Haiku and Sonnet

Haiku output now includes a "triage" field ("evaluate" or "clean").
Pipeline skips the Sonnet evaluator for sessions Haiku classifies as
clean, saving Sonnet invocation cost. Defaults to "evaluate" for
backward compatibility with v2 prompt output.
```

---

### Task 4: Switch Haiku to agentic mode

**Files:**
- Modify: `internal/pipeline/haiku.go:14-64` (agentic invocation)
- Modify: `internal/pipeline/prompts.go:38-147` (v3 prompt)
- Modify: `internal/pipeline/prompts.go:19-23` (add v3 to EnsurePrompts)

**Step 1: Update prompt file constant**

```go
const haikuPromptFile = "haiku-classifier-v3.txt"
```

**Step 2: Write v3 prompt**

Replace `defaultHaikuPrompt` with v3 that includes:

1. All existing classification instructions (goal, errors, key turns,
   skill signals, CLAUDE.md signals, pattern assessments)
2. New triage field in output schema:
   ```json
   "triage": "evaluate|clean"
   ```
   With guidance: "Set to 'clean' if the session has no actionable
   signals — no skill friction, no CLAUDE.md issues, no confirmed
   cross-session patterns, no ambiguous signals worth investigating.
   Set to 'evaluate' if ANY signal warrants deeper analysis by the
   Sonnet evaluator."
3. Tool usage guidance:
   ```
   ## Tool access

   You have Read and Grep tools available for reading files under
   ~/.cabrero/raw/. Use them to verify ambiguous signals by reading
   the surrounding raw JSONL turns.

   The digest provides UUIDs for every signal. To inspect a signal's
   context, Grep for the UUID in the session's transcript file:
     ~/.cabrero/raw/{sessionId}/transcript.jsonl

   Situations that often benefit from reading raw turns:
   - Friction signals near threshold (2 fumbles instead of 3)
   - Errors where attribution is ambiguous (tool failure vs user input)
   - Skill read with unclear impact (what happened after loading?)
   - Sub-agent marked abandoned (was it actually?)
   - Sparse sessions with few signals (blind spots?)
   - High completion + high friction (succeeded despite problems)

   You decide when to use tools. Not every signal needs verification —
   clear-cut signals can be classified from the digest alone.

   Do NOT read files outside ~/.cabrero/raw/.
   ```
4. Bump prompt version to `haiku-classifier-v3`
5. Bump output schema version to 2

**Step 3: Update RunHaiku to use agentic mode**

```go
func RunHaiku(sessionID string, digest *parser.Digest, aggregatorOutput *patterns.AggregatorOutput, cfg PipelineConfig) (*HaikuOutput, error) {
	systemPrompt, err := readPromptTemplate(haikuPromptFile)
	if err != nil {
		return nil, fmt.Errorf("reading haiku prompt: %w", err)
	}

	digestJSON, err := json.MarshalIndent(digest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling digest: %w", err)
	}

	// Build user prompt with digest and optional cross-session patterns.
	prompt := "<session_digest>\n" + string(digestJSON) + "\n</session_digest>"

	if aggregatorOutput != nil && len(aggregatorOutput.Patterns) > 0 {
		patternsJSON, err := json.MarshalIndent(aggregatorOutput, "", "  ")
		if err == nil {
			prompt += "\n\n<cross_session_patterns>\n" + string(patternsJSON) + "\n</cross_session_patterns>"
		}
	}

	stdout, err := invokeClaude(claudeConfig{
		Model:        "claude-haiku-4-5",
		SystemPrompt: systemPrompt,
		Agentic:      true,
		Prompt:       prompt,
		AllowedTools: "Read,Grep",
		MaxTurns:     cfg.HaikuMaxTurns,
		Timeout:      cfg.HaikuTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("invoking haiku: %w", err)
	}

	// Parse, validate, return (unchanged from here).
	// ...
}
```

**Step 4: Update EnsurePrompts**

Add v3 to the prompts map:

```go
prompts := map[string]string{
	haikuPromptFile:  defaultHaikuPrompt,
	sonnetPromptFile: defaultSonnetPrompt,
}
```

This already works because `haikuPromptFile` is now `"haiku-classifier-v3.txt"`.

**Step 5: Build and test manually**

```bash
make build
# Run against a known session to verify agentic Haiku works:
./cabrero run <session_id> --dry-run  # pre-parser only, safe
# Then without --dry-run on a test session if available
```

**Step 6: Commit**

```
feat(pipeline): switch Haiku classifier to agentic mode

Haiku now runs as a tool-using session with Read/Grep access scoped to
~/.cabrero/raw/ (via prompt instruction). Receives digest as -p prompt
instead of stdin pipe. Can verify ambiguous signals by reading raw JSONL
turns. Adds triage field to output — sessions classified as "clean"
skip the Sonnet evaluator. Prompt bumped to v3.
```

---

### Task 5: Switch Sonnet to agentic mode

**Files:**
- Modify: `internal/pipeline/sonnet.go:13-61` (agentic invocation)
- Modify: `internal/pipeline/prompts.go:149-227` (v3 prompt)

**Step 1: Update prompt file constant**

```go
const sonnetPromptFile = "sonnet-evaluator-v3.txt"
```

**Step 2: Write v3 prompt**

Replace `defaultSonnetPrompt` with v3 that includes:

1. All existing proposal generation instructions
2. Tool usage guidance:
   ```
   ## Tool access

   You have Read and Grep tools with unrestricted filesystem access.
   Use them to:

   - Read current versions of skill files referenced in the digest
     to understand what guidance they provide
   - Read CLAUDE.md files to understand active instructions
   - Read raw JSONL turns from ~/.cabrero/raw/{sessionId}/ to verify
     signals and gather evidence for proposals
   - Compare current file content against what the session transcript
     shows (skill content may have changed since the session)

   When generating proposals, READ the target file first. A proposal
   to improve a skill should be informed by the skill's current content,
   not just the Haiku classification. A CLAUDE.md review flag should
   reference the actual entry text.

   You have full read access. Use it to make better-informed proposals.
   ```
3. Bump prompt version to `sonnet-evaluator-v3`
4. Bump output schema version to 2

**Step 3: Update RunSonnet to use agentic mode**

```go
func RunSonnet(sessionID string, digest *parser.Digest, haikuOutput *HaikuOutput, cfg PipelineConfig) (*SonnetOutput, error) {
	systemPrompt, err := readPromptTemplate(sonnetPromptFile)
	if err != nil {
		return nil, fmt.Errorf("reading sonnet prompt: %w", err)
	}

	haikuJSON, err := json.MarshalIndent(haikuOutput, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling haiku output: %w", err)
	}

	digestJSON, err := json.MarshalIndent(digest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling digest: %w", err)
	}

	prompt := "<haiku_classification>\n" + string(haikuJSON) + "\n</haiku_classification>" +
		"\n\n<session_digest>\n" + string(digestJSON) + "\n</session_digest>"

	stdout, err := invokeClaude(claudeConfig{
		Model:        "claude-sonnet-4-6",
		SystemPrompt: systemPrompt,
		Effort:       "high",
		Agentic:      true,
		Prompt:       prompt,
		AllowedTools: "Read,Grep",
		MaxTurns:     cfg.SonnetMaxTurns,
		Timeout:      cfg.SonnetTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("invoking sonnet: %w", err)
	}

	// Parse, validate, return (unchanged from here).
	// ...
}
```

**Step 4: Build and test**

```bash
make build
```

**Step 5: Commit**

```
feat(pipeline): switch Sonnet evaluator to agentic mode

Sonnet now runs as a tool-using session with unrestricted Read/Grep
access. Can read current skill files, CLAUDE.md content, and raw
session data to make better-informed proposals. Receives digest +
Haiku output as -p prompt. Prompt bumped to v3.
```

---

### Task 6: Smart batching in daemon loop

**Files:**
- Modify: `internal/pipeline/pipeline.go` (add RunBatch function)
- Modify: `internal/pipeline/sonnet.go` (add RunSonnetBatch)
- Modify: `internal/daemon/daemon.go:92-124` (rewrite processPending)
- Modify: `internal/daemon/scanner.go` (return metadata with sessions)

**Step 1: Update ScanPending to return project metadata**

Currently `ScanPending()` returns `[]string` (session IDs). Update to
return session ID + project so the daemon can group:

```go
type PendingSession struct {
	SessionID string
	Project   string
}

func ScanPending() ([]PendingSession, error) {
```

Read metadata for each pending session to extract `Project`. Sessions
without project metadata go into a "" (empty) project group and are
processed individually.

**Step 2: Add RunSonnetBatch**

New function that accepts multiple sessions' data in one Sonnet call:

```go
type BatchSession struct {
	SessionID        string
	Digest           *parser.Digest
	HaikuOutput      *HaikuOutput
	AggregatorOutput *patterns.AggregatorOutput
}

func RunSonnetBatch(sessions []BatchSession, cfg PipelineConfig) (*SonnetOutput, error) {
```

The prompt wraps each session's data in indexed tags:

```
<session index="1" id="abc123">
  <haiku_classification>...</haiku_classification>
  <session_digest>...</session_digest>
</session>

<session index="2" id="def456">
  <haiku_classification>...</haiku_classification>
  <session_digest>...</session_digest>
</session>
```

Sonnet's v3 prompt already has unrestricted Read/Grep — it can read all
sessions' raw data. The batch prompt adds:

```
You are evaluating multiple sessions from the same project. You may
find cross-session patterns that strengthen or weaken individual
signals. Generate proposals per-session (each proposal's citedUuids
reference one session) but use cross-session context to inform
confidence levels.
```

Output schema stays the same — proposals still reference individual
session IDs. The batch just gives Sonnet richer context.

Max turns scale with batch size: `cfg.SonnetMaxTurns * len(sessions)`.
More sessions means more files to read and more signals to investigate.
The timeout scales similarly: `cfg.SonnetTimeout * time.Duration(len(sessions))`.
Both are capped at reasonable maximums (e.g., 60 turns, 15 min) to
prevent runaway batches.

**Step 3: Rewrite daemon processPending**

```go
func (d *Daemon) processPending(ctx context.Context) {
	pending, err := ScanPending()
	if err != nil {
		d.log.Error("scanning pending sessions: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	d.log.Info("found %d pending session(s)", len(pending))

	// Group by project.
	byProject := make(map[string][]PendingSession)
	for _, p := range pending {
		byProject[p.Project] = append(byProject[p.Project], p)
	}

	for project, sessions := range byProject {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if project == "" {
			// No project metadata — process individually.
			for _, s := range sessions {
				d.processOne(s.SessionID)
			}
			continue
		}

		d.processProjectBatch(ctx, project, sessions)

		if d.config.InterSessionDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d.config.InterSessionDelay):
			}
		}
	}
}
```

**Step 4: Add processProjectBatch**

```go
func (d *Daemon) processProjectBatch(ctx context.Context, project string, sessions []PendingSession) {
	d.log.Info("processing %d session(s) for project %s", len(sessions), project)

	// Phase 1: Run Haiku individually on each session.
	var toEvaluate []pipeline.BatchSession
	for _, s := range sessions {
		result, err := pipeline.RunThroughHaiku(s.SessionID, d.config.Pipeline)
		if err != nil {
			d.log.Error("haiku failed for %s: %v", s.SessionID, err)
			d.markError(s.SessionID)
			continue
		}

		if result.HaikuOutput.Triage == "clean" {
			d.log.Info("session %s triaged as clean — skipping evaluator", shortID(s.SessionID))
			d.markProcessed(s.SessionID)
			continue
		}

		toEvaluate = append(toEvaluate, pipeline.BatchSession{
			SessionID:        s.SessionID,
			Digest:           result.Digest,
			HaikuOutput:      result.HaikuOutput,
			AggregatorOutput: result.AggregatorOutput,
		})
	}

	if len(toEvaluate) == 0 {
		return
	}

	// Phase 2: Run Sonnet — batch if 2+, individual if 1.
	if len(toEvaluate) == 1 {
		d.runSonnetSingle(toEvaluate[0])
	} else {
		d.runSonnetBatch(toEvaluate)
	}
}
```

**Step 5: Add RunThroughHaiku**

New pipeline function that runs pre-parser + aggregator + Haiku only
(everything before Sonnet). Returns enough data for batching:

```go
type HaikuResult struct {
	Digest           *parser.Digest
	AggregatorOutput *patterns.AggregatorOutput
	HaikuOutput      *HaikuOutput
}

func RunThroughHaiku(sessionID string, cfg PipelineConfig) (*HaikuResult, error) {
```

This extracts the first half of the existing `Run()` function. The
existing `Run()` function calls `RunThroughHaiku` internally, then
continues to Sonnet. `cabrero run` still uses `Run()` for individual
sessions.

**Step 6: Build and verify**

```bash
make build
```

**Step 7: Commit**

```
feat(daemon): smart batch Sonnet invocations per project

When multiple pending sessions belong to the same project, the daemon
runs Haiku individually on each (cheap triage), then batches sessions
Haiku flagged as "evaluate" into a single Sonnet invocation. Gives
Sonnet cross-session context within one call while keeping Haiku
independent. Sessions without project metadata or solo sessions for
a project fall back to individual processing.
```

---

### Task 7: Add daemon CLI flags for evaluator config

**Files:**
- Modify: `internal/cmd/daemon.go:16-28` (new flags)
- Modify: `internal/cmd/run.go` (new flags)

**Step 1: Add flags to daemon command**

```go
func Daemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	poll := fs.Duration("poll", 2*time.Minute, "how often to check for pending sessions")
	stale := fs.Duration("stale", 30*time.Minute, "how often to scan for stale sessions")
	delay := fs.Duration("delay", 30*time.Second, "pause between processing sessions")
	haikuMaxTurns := fs.Int("haiku-max-turns", 15, "max agentic turns for Haiku classifier")
	sonnetMaxTurns := fs.Int("sonnet-max-turns", 20, "max agentic turns for Sonnet evaluator")
	haikuTimeout := fs.Duration("haiku-timeout", 2*time.Minute, "timeout for Haiku classifier")
	sonnetTimeout := fs.Duration("sonnet-timeout", 5*time.Minute, "timeout for Sonnet evaluator")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := daemon.DefaultConfig()
	cfg.PollInterval = *poll
	cfg.StaleInterval = *stale
	cfg.InterSessionDelay = *delay
	cfg.Pipeline.HaikuMaxTurns = *haikuMaxTurns
	cfg.Pipeline.SonnetMaxTurns = *sonnetMaxTurns
	cfg.Pipeline.HaikuTimeout = *haikuTimeout
	cfg.Pipeline.SonnetTimeout = *sonnetTimeout
	// ...
}
```

**Step 2: Add flags to run command**

Same flags for `cabrero run` so manual pipeline runs can override:

```go
func Run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "only run pre-parser")
	haikuMaxTurns := fs.Int("haiku-max-turns", 15, "max agentic turns for Haiku")
	sonnetMaxTurns := fs.Int("sonnet-max-turns", 20, "max agentic turns for Sonnet")
	// ...

	cfg := pipeline.DefaultPipelineConfig()
	cfg.HaikuMaxTurns = *haikuMaxTurns
	cfg.SonnetMaxTurns = *sonnetMaxTurns

	result, err := pipeline.Run(sessionID, *dryRun, cfg)
	// ...
}
```

**Step 3: Build and verify**

```bash
make build
./cabrero daemon --help
./cabrero run --help
```

**Step 4: Commit**

```
feat(daemon): add CLI flags for agentic evaluator limits

--haiku-max-turns, --sonnet-max-turns, --haiku-timeout,
--sonnet-timeout configurable on both `cabrero daemon` and
`cabrero run` commands. Defaults: Haiku 15 turns / 2 min,
Sonnet 20 turns / 5 min.
```

---

### Task 8: Update DESIGN.md CLI subcommands

**Files:**
- Modify: `DESIGN.md` (daemon and run subcommand docs)

**Step 1: Update daemon subcommand docs**

Add new flags to the daemon entry in the CLI subcommands section:

```
cabrero daemon                  Run background session processor (for launchd)
  --poll <duration>               Pending session check interval (default 2m)
  --stale <duration>              Stale session scan interval (default 30m)
  --delay <duration>              Pause between processing sessions (default 30s)
  --haiku-max-turns <int>         Max agentic turns for Haiku (default 15)
  --sonnet-max-turns <int>        Max agentic turns for Sonnet (default 20)
  --haiku-timeout <duration>      Timeout for Haiku classifier (default 2m)
  --sonnet-timeout <duration>     Timeout for Sonnet evaluator (default 5m)
```

**Step 2: Commit**

```
docs: add agentic evaluator flags to CLI subcommands
```

---

### Task 9: Update CHANGELOG.md

**Files:**
- Modify: `CHANGELOG.md`

**Step 1: Add entry under [Unreleased]**

```markdown
- **Agentic evaluators** — Haiku classifier and Sonnet evaluator now run
  as tool-using sessions with Read/Grep access instead of single-shot
  `--print` invocations. Haiku can verify ambiguous signals by reading
  raw JSONL turns (scoped to `~/.cabrero/raw/`). Sonnet can read current
  skill files, CLAUDE.md content, and session data for better-informed
  proposals. New triage gate skips Sonnet for sessions Haiku classifies
  as clean. Configurable via `--haiku-max-turns`, `--sonnet-max-turns`,
  `--haiku-timeout`, `--sonnet-timeout` on both `daemon` and `run`
  commands. Prompts bumped to v3.
```

**Step 2: Commit**

```
docs: add agentic evaluators to changelog
```

---

## Summary

| Task | What changes | Risk |
|------|-------------|------|
| 1 | PipelineConfig struct, thread through all callers | Low — no behavior change |
| 2 | invokeClaude dual mode (--print vs -p) | Medium — core invocation change |
| 3 | Triage field + pipeline gate | Low — backward compatible default |
| 4 | Haiku agentic + v3 prompt | High — changes LLM behavior |
| 5 | Sonnet agentic + v3 prompt | High — changes LLM behavior |
| 6 | Smart batching in daemon loop | Medium — daemon orchestration rewrite |
| 7 | Daemon/run CLI flags | Low — additive |
| 8 | DESIGN.md | Low — docs only |
| 9 | CHANGELOG.md | Low — docs only |

Tasks 4 and 5 are the high-risk changes. Test them against known sessions
before deploying to the daemon. The `cabrero run <session_id>` command is
the right way to test — run against a previously processed session and
compare output quality.

Task 6 rewrites the daemon's processing loop. Test with multiple pending
sessions from the same project to verify batching works, and with mixed
projects to verify individual processing still works.

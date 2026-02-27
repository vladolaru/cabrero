package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vladolaru/cabrero/internal/store"
)

// maxScanBuffer is 10 MB — handles the largest observed JSONL lines (100KB+).
const maxScanBuffer = 10 * 1024 * 1024

// retryWindowSeconds is the maximum gap between tool calls to consider them a retry.
const retryWindowSeconds = 60.0

// knownEntryTypes lists all recognised JSONL entry types.
var knownEntryTypes = map[string]bool{
	"user":                  true,
	"assistant":             true,
	"progress":              true,
	"system":                true,
	"summary":               true,
	"file-history-snapshot": true,
	"queue-operation":       true,
}

// claudeMdPathRe matches file paths ending in CLAUDE.md (with optional leading path).
var claudeMdPathRe = regexp.MustCompile(`(?:^|/)([^\s"]+/CLAUDE\.md|CLAUDE\.md)`)

// claudeMdContentsRe matches the legacy "Contents of" pattern from system-reminder tags.
var claudeMdContentsRe = regexp.MustCompile(`Contents of ([^\n]+CLAUDE\.md)`)

// rawEntry is a minimal struct for initial line parsing.
// Only fields common across all entry types are decoded here.
type rawEntry struct {
	Type      string          `json:"type"`
	UUID      *string         `json:"uuid"`
	Timestamp *string         `json:"timestamp"`
	SessionID *string         `json:"sessionId"`
	Version   *string         `json:"version"`
	UserType  *string         `json:"userType"`
	IsMeta    *bool           `json:"isMeta"`
	AgentID   *string         `json:"agentId"`
	Message   json.RawMessage `json:"message"`
	Cwd       *string         `json:"cwd"`
	GitBranch *string         `json:"gitBranch"`

	// summary-specific
	Summary  *string `json:"summary"`
	LeafUUID *string `json:"leafUuid"`

	// system-specific
	Subtype    *string `json:"subtype"`
	DurationMs *int64  `json:"durationMs"`
	ParentUUID *string `json:"parentUuid"`

	// queue-operation-specific
	Operation *string `json:"operation"`

	// progress-specific
	Data json.RawMessage `json:"data"`

	// file-history-snapshot-specific
	Snapshot json.RawMessage `json:"snapshot"`

	// tool result convenience fields
	ToolUseResult             json.RawMessage `json:"toolUseResult"`
	SourceToolAssistantUUID   *string         `json:"sourceToolAssistantUUID"`
}

// messageEnvelope extracts role and content from the message field.
type messageEnvelope struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	ID      string          `json:"id"`
	Model   *string         `json:"model"`
	Usage   *usageBlock     `json:"usage"`
}

type usageBlock struct {
	InputTokens              int64        `json:"input_tokens"`
	OutputTokens             int64        `json:"output_tokens"`
	CacheCreationInputTokens int64        `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64        `json:"cache_read_input_tokens"`
	CacheCreation            *cacheDetail `json:"cache_creation"`
}

type cacheDetail struct {
	Ephemeral5mInputTokens int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int64 `json:"ephemeral_1h_input_tokens"`
}

// ParseSession reads a transcript JSONL file and produces a Digest.
func ParseSession(sessionID string) (*Digest, error) {
	rawDir := store.RawDir(sessionID)
	transcriptPath := filepath.Join(rawDir, "transcript.jsonl")

	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil, fmt.Errorf("opening transcript: %w", err)
	}
	defer f.Close()

	d := &Digest{
		Version:     1,
		SessionID:   sessionID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		ToolCalls: ToolCalls{
			Summary:         make(map[string]ToolCallDetail),
			RetryAnomalies:  []RetryAnomaly{},
			FrictionSignals: []FrictionSignal{},
		},
		Agents: AgentsInfo{
			Inventory: []AgentInventoryItem{},
		},
		RawUnknown: []RawUnknown{},
	}

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, maxScanBuffer)

	var (
		lineNum          int
		modelsSet        = make(map[string]bool)
		agentIDCounts    = make(map[string]int)
		agentSpawns      = make(map[string]*AgentInventoryItem) // toolUseID → item
		agentResultIDs   = make(map[string]bool)                // toolUseIDs that got results
		segmentStart     int                                    // line index of current segment start
		segmentFirstTS   *string
		segmentLastTS    *string
		recentToolCalls  []recentToolCall // sliding window for retry detection
		lastToolName     *string
		skillToolUseIDs  = make(map[string]*SkillEntry) // tool_use_id → skill entry
		toolUseIDToName  = make(map[string]string)      // tool_use_id → tool name (for error attribution)
		claudeMdSeen     = make(map[string]bool)        // path → already recorded
		fileAccessLog    []fileAccess                    // ordered file accesses for backtrack detection
		seqIndex         int                             // global sequence counter for tool calls
	)

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		var entry rawEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			d.RawUnknown = append(d.RawUnknown, RawUnknown{
				Type:       "parse_error",
				LineNumber: lineNum,
			})
			continue
		}

		d.Shape.EntryCount++

		// Track timestamps for segment tracking.
		if entry.Timestamp != nil {
			if segmentFirstTS == nil {
				segmentFirstTS = entry.Timestamp
			}
			segmentLastTS = entry.Timestamp
		}

		// CC version and cwd from first entry that has them.
		if d.Shape.CCVersion == nil && entry.Version != nil {
			v := *entry.Version
			d.Shape.CCVersion = &v
		}
		if d.Shape.Cwd == nil && entry.Cwd != nil {
			cwd := *entry.Cwd
			d.Shape.Cwd = &cwd
		}

		// Track global timestamps.
		if entry.Timestamp != nil {
			if d.Shape.FirstTimestamp == nil {
				ts := *entry.Timestamp
				d.Shape.FirstTimestamp = &ts
			}
			ts := *entry.Timestamp
			d.Shape.LastTimestamp = &ts
		}

		// Track agent IDs in main transcript.
		if entry.AgentID != nil {
			agentIDCounts[*entry.AgentID]++
		}

		if !knownEntryTypes[entry.Type] {
			d.RawUnknown = append(d.RawUnknown, RawUnknown{
				Type:       entry.Type,
				UUID:       entry.UUID,
				Timestamp:  entry.Timestamp,
				LineNumber: lineNum,
			})
			continue
		}

		switch entry.Type {
		case "user":
			d.processUser(&entry, modelsSet, agentResultIDs, skillToolUseIDs, toolUseIDToName, claudeMdSeen)

		case "assistant":
			d.processAssistant(&entry, modelsSet, agentSpawns, &recentToolCalls, &lastToolName, skillToolUseIDs, toolUseIDToName, claudeMdSeen, &fileAccessLog, &seqIndex)

		case "summary":
			seg := CompactionSegment{
				Index:            len(d.CompactionSegments),
				EntryCountBefore: d.Shape.EntryCount - 1 - segmentStart,
				FirstTimestamp:   segmentFirstTS,
				LastTimestamp:     segmentLastTS,
			}
			if entry.Summary != nil {
				seg.SummaryText = *entry.Summary
			}
			if entry.LeafUUID != nil {
				seg.LeafUUID = *entry.LeafUUID
			}
			d.CompactionSegments = append(d.CompactionSegments, seg)
			d.Shape.CompactionCount++

			// Reset segment tracking.
			segmentStart = d.Shape.EntryCount
			segmentFirstTS = nil
			segmentLastTS = nil

		case "system":
			if entry.Subtype != nil && *entry.Subtype == "turn_duration" && entry.DurationMs != nil {
				td := TurnDuration{
					DurationMs: *entry.DurationMs,
				}
				if entry.UUID != nil {
					td.UUID = *entry.UUID
				}
				if entry.ParentUUID != nil {
					td.ParentUUID = *entry.ParentUUID
				}
				d.TurnDurations = append(d.TurnDurations, td)
			}

		case "queue-operation":
			if entry.Operation != nil {
				switch *entry.Operation {
				case "enqueue":
					d.Completion.QueueEnqueueCount++
				case "dequeue":
					d.Completion.QueueDequeueCount++
				}
			}

		case "progress":
			// Progress entries tracked only for agent detection.
			d.processProgress(&entry)

		case "file-history-snapshot":
			d.processFileSnapshot(&entry, claudeMdSeen)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning transcript: %w", err)
	}

	// Compute duration.
	if d.Shape.FirstTimestamp != nil && d.Shape.LastTimestamp != nil {
		t1, err1 := time.Parse(time.RFC3339, *d.Shape.FirstTimestamp)
		t2, err2 := time.Parse(time.RFC3339, *d.Shape.LastTimestamp)
		if err1 == nil && err2 == nil {
			dur := t2.Sub(t1).Seconds()
			d.Shape.DurationSeconds = &dur
		}
	}

	// Finalize models list.
	for m := range modelsSet {
		d.Shape.Models = append(d.Shape.Models, m)
	}

	// Compute cache hit ratio.
	totalCache := d.Shape.TokenUsage.TotalCacheReadTokens + d.Shape.TokenUsage.TotalCacheCreationTokens
	if totalCache > 0 {
		ratio := float64(d.Shape.TokenUsage.TotalCacheReadTokens) / float64(totalCache)
		d.Shape.CacheHitRatio = &ratio
	}

	// Finalize agents.
	d.finalizeAgents(sessionID, agentSpawns, agentResultIDs, agentIDCounts)

	// Detect retry anomalies and friction signals.
	d.ToolCalls.RetryAnomalies = detectRetryAnomalies(recentToolCalls)
	d.ToolCalls.FrictionSignals = append(d.ToolCalls.FrictionSignals, detectSearchFumbles(recentToolCalls)...)
	d.ToolCalls.FrictionSignals = append(d.ToolCalls.FrictionSignals, detectBacktracking(fileAccessLog)...)

	// Set last tool name.
	d.Completion.LastToolName = lastToolName

	// Skills are already populated in d.Skills via processSkillInvocation.
	// ResultUUID fields were patched in processUser via skillToolUseIDs map.

	// Infer which CLAUDE.md files were loaded into the session context.
	d.inferClaudeMdLoaded()

	return d, nil
}

func (d *Digest) processUser(entry *rawEntry, modelsSet map[string]bool, agentResultIDs map[string]bool, skillToolUseIDs map[string]*SkillEntry, toolUseIDToName map[string]string, claudeMdSeen map[string]bool) {
	if len(entry.Message) == 0 {
		return
	}

	var msg messageEnvelope
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return
	}

	blocks := parseContentBlocks(msg.Content)

	// Count user turns: userType=external, not isMeta, not just tool results.
	isExternal := entry.UserType != nil && *entry.UserType == "external"
	isMeta := entry.IsMeta != nil && *entry.IsMeta
	onlyResults := isOnlyToolResults(blocks)

	if isExternal && !isMeta && !onlyResults {
		d.Shape.TurnCount++
	}

	uuid := ""
	if entry.UUID != nil {
		uuid = *entry.UUID
	}
	ts := ""
	if entry.Timestamp != nil {
		ts = *entry.Timestamp
	}

	// Extract CLAUDE.md references from text content (legacy "Contents of" pattern).
	textContent := extractTextContent(blocks)
	matches := claudeMdContentsRe.FindAllStringSubmatch(textContent, -1)
	for _, m := range matches {
		d.addClaudeMdInteraction(m[1], uuid, ts, "text", claudeMdSeen)
	}

	// Detect CLAUDE.md from toolUseResult convenience field (filePath).
	if len(entry.ToolUseResult) > 0 {
		d.detectClaudeMdInToolResult(entry.ToolUseResult, uuid, ts, claudeMdSeen)
	}

	// Process tool_result blocks.
	toolResults := extractToolResultBlocks(blocks)
	for _, tr := range toolResults {
		// Track agent result IDs for abandoned detection.
		if tr.ToolUseID != "" {
			agentResultIDs[tr.ToolUseID] = true
		}

		// Track skill results.
		if se, ok := skillToolUseIDs[tr.ToolUseID]; ok {
			u := uuid
			se.ResultUUID = &u
		}

		// Detect CLAUDE.md in tool_result content.
		if len(tr.Content) > 0 {
			d.detectClaudeMdInToolResult(tr.Content, uuid, ts, claudeMdSeen)
		}

		// Track errors.
		if tr.IsError {
			errEntry := ErrorEntry{
				UUID:      uuid,
				ToolUseID: tr.ToolUseID,
				Snippet:   toolResultSnippet(tr.Content),
				Timestamp: ts,
			}

			// Look up tool name from source assistant UUID.
			if entry.SourceToolAssistantUUID != nil {
				errEntry.SourceAssistantUUID = entry.SourceToolAssistantUUID
			}

			// Attribute error to tool name and increment ErrorCount.
			if tn, ok := toolUseIDToName[tr.ToolUseID]; ok {
				errEntry.ToolName = &tn
				detail := d.ToolCalls.Summary[tn]
				detail.ErrorCount++
				d.ToolCalls.Summary[tn] = detail
			}

			d.Errors = append(d.Errors, errEntry)
		}

		// Detect empty search results (non-error tool_results from search tools).
		if !tr.IsError && tr.ToolUseID != "" {
			if tn, ok := toolUseIDToName[tr.ToolUseID]; ok {
				if isSearchTool(tn) && isEmptySearchResult(tr.Content) {
					d.ToolCalls.FrictionSignals = append(d.ToolCalls.FrictionSignals, FrictionSignal{
						Type:      "empty_search",
						ToolName:  tn,
						UUIDs:     []string{uuid},
						Detail:    "search returned no results",
						Timestamp: ts,
					})
				}
			}
		}
	}
}

func (d *Digest) processAssistant(entry *rawEntry, modelsSet map[string]bool, agentSpawns map[string]*AgentInventoryItem, recentToolCalls *[]recentToolCall, lastToolName **string, skillToolUseIDs map[string]*SkillEntry, toolUseIDToName map[string]string, claudeMdSeen map[string]bool, fileAccessLog *[]fileAccess, seqIndex *int) {
	if len(entry.Message) == 0 {
		return
	}

	var msg messageEnvelope
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return
	}

	// Track model.
	if msg.Model != nil && *msg.Model != "" {
		modelsSet[*msg.Model] = true
	}

	// Accumulate token usage.
	if msg.Usage != nil {
		d.Shape.TokenUsage.TotalInputTokens += msg.Usage.InputTokens
		d.Shape.TokenUsage.TotalOutputTokens += msg.Usage.OutputTokens
		d.Shape.TokenUsage.TotalCacheCreationTokens += msg.Usage.CacheCreationInputTokens
		d.Shape.TokenUsage.TotalCacheReadTokens += msg.Usage.CacheReadInputTokens
		if msg.Usage.CacheCreation != nil {
			d.Shape.TokenUsage.TotalCacheCreationTokens += msg.Usage.CacheCreation.Ephemeral5mInputTokens
			d.Shape.TokenUsage.TotalCacheCreationTokens += msg.Usage.CacheCreation.Ephemeral1hInputTokens
		}
	}

	blocks := parseContentBlocks(msg.Content)
	toolUses := extractToolUseBlocks(blocks)

	uuid := ""
	if entry.UUID != nil {
		uuid = *entry.UUID
	}
	ts := ""
	if entry.Timestamp != nil {
		ts = *entry.Timestamp
	}

	for _, tu := range toolUses {
		toolName := tu.Name

		// Record tool_use_id → tool_name mapping for error attribution.
		if tu.ID != "" {
			toolUseIDToName[tu.ID] = toolName
		}

		// Update tool call summary.
		detail := d.ToolCalls.Summary[toolName]
		detail.Count++
		if detail.FirstUUID == "" {
			detail.FirstUUID = uuid
		}
		detail.LastUUID = uuid
		d.ToolCalls.Summary[toolName] = detail

		// Track last tool name.
		tn := toolName
		*lastToolName = &tn

		// Track for retry anomaly detection.
		*recentToolCalls = append(*recentToolCalls, recentToolCall{
			toolName:  toolName,
			uuid:      uuid,
			timestamp: ts,
			inputKey:  toolUseInputString(tu.Input),
		})

		// Track agent spawns (Task tool).
		if toolName == "Task" {
			d.processAgentSpawn(tu, uuid, agentSpawns)
		}

		// Track skill invocations.
		if toolName == "Skill" {
			d.processSkillInvocation(tu, uuid, ts, skillToolUseIDs)
		}

		// Track completion signals.
		d.processCompletionSignals(toolName, tu)

		// Detect CLAUDE.md in tool input file paths.
		d.detectClaudeMdInToolUse(tu, uuid, ts, claudeMdSeen)

		// Track file accesses for backtracking detection.
		if isFileAccessTool(toolName) {
			if fp := extractFilePath(tu.Input); fp != "" {
				*fileAccessLog = append(*fileAccessLog, fileAccess{
					filePath: fp,
					toolName: toolName,
					uuid:     uuid,
					seqIndex: *seqIndex,
				})
			}
		}
		(*seqIndex)++
	}
}

func (d *Digest) processProgress(entry *rawEntry) {
	// Progress entries are acknowledged; their data contributes to agent detection
	// via the agentId field (already tracked in the main loop).
}

func (d *Digest) processAgentSpawn(tu contentBlock, uuid string, agentSpawns map[string]*AgentInventoryItem) {
	var input struct {
		SubagentType *string `json:"subagent_type"`
		Description  *string `json:"description"`
	}
	_ = json.Unmarshal(tu.Input, &input)

	item := &AgentInventoryItem{
		AgentID:      tu.ID, // tool_use ID serves as initial agent reference
		ParentUUID:   uuid,
		ToolName:     "Task",
		SubagentType: input.SubagentType,
		Description:  input.Description,
	}
	agentSpawns[tu.ID] = item
}

func (d *Digest) processSkillInvocation(tu contentBlock, uuid, ts string, skillToolUseIDs map[string]*SkillEntry) {
	var input struct {
		Skill string `json:"skill"`
	}
	_ = json.Unmarshal(tu.Input, &input)

	se := &SkillEntry{
		SkillName:     input.Skill,
		InvokedAtUUID: uuid,
		Timestamp:     ts,
	}
	d.Skills = append(d.Skills, *se)
	skillToolUseIDs[tu.ID] = &d.Skills[len(d.Skills)-1]
}

func (d *Digest) processCompletionSignals(toolName string, tu contentBlock) {
	switch toolName {
	case "TaskCreate":
		d.Completion.TaskCreateCount++
	case "TaskUpdate":
		var input struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(tu.Input, &input); err == nil {
			if input.Status == "completed" {
				d.Completion.TaskUpdateCompletedCount++
			} else if input.Status != "" {
				d.Completion.TaskUpdatePendingCount++
			}
		}
	case "Bash":
		var input struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(tu.Input, &input); err == nil {
			cmd := strings.ToLower(input.Command)
			if strings.Contains(cmd, "git diff") || strings.Contains(cmd, "git commit") || strings.Contains(cmd, "git push") {
				d.Completion.GitDiffPresent = true
			}
		}
	}
}

// inferClaudeMdLoaded populates the ClaudeMd.Loaded list based on the session's cwd.
// CC injects CLAUDE.md from two locations into every session's system prompt:
//   - ~/.claude/CLAUDE.md (user-level, always loaded if it exists)
//   - {project_root}/CLAUDE.md (project-level, loaded if cwd is in a project)
func (d *Digest) inferClaudeMdLoaded() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// User-level CLAUDE.md — always loaded by CC.
	userClaudeMd := filepath.Join(home, ".claude", "CLAUDE.md")
	d.ClaudeMd.Loaded = append(d.ClaudeMd.Loaded, ClaudeMdLoaded{
		Path:   userClaudeMd,
		Source: "user_config",
	})

	// Project-level CLAUDE.md — inferred from the session's working directory.
	if d.Shape.Cwd != nil {
		cwd := *d.Shape.Cwd
		projectClaudeMd := filepath.Join(cwd, "CLAUDE.md")
		if projectClaudeMd != userClaudeMd {
			d.ClaudeMd.Loaded = append(d.ClaudeMd.Loaded, ClaudeMdLoaded{
				Path:   projectClaudeMd,
				Source: "project_cwd",
			})
		}
	}
}

// addClaudeMdInteraction records an explicit CLAUDE.md file interaction, deduplicating by path.
func (d *Digest) addClaudeMdInteraction(path, uuid, ts, source string, seen map[string]bool) {
	if seen[path] {
		return
	}
	seen[path] = true
	d.ClaudeMd.Interactions = append(d.ClaudeMd.Interactions, ClaudeMdInteraction{
		Path:        path,
		FoundInUUID: uuid,
		Timestamp:   ts,
		Source:      source,
	})
}

// detectClaudeMdInToolUse checks tool_use input for file_path/path arguments ending in CLAUDE.md.
func (d *Digest) detectClaudeMdInToolUse(tu contentBlock, uuid, ts string, seen map[string]bool) {
	if len(tu.Input) == 0 {
		return
	}
	var input struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal(tu.Input, &input); err != nil {
		return
	}
	for _, p := range []string{input.FilePath, input.Path} {
		if p != "" && isClaudeMdPath(p) {
			d.addClaudeMdInteraction(p, uuid, ts, "tool_use", seen)
		}
	}
}

// detectClaudeMdInToolResult checks tool_result content for CLAUDE.md file paths.
func (d *Digest) detectClaudeMdInToolResult(raw json.RawMessage, uuid, ts string, seen map[string]bool) {
	// Try structured form with filePath field.
	var structured struct {
		FilePath string `json:"filePath"`
		File     struct {
			FilePath string `json:"filePath"`
		} `json:"file"`
	}
	if err := json.Unmarshal(raw, &structured); err == nil {
		for _, p := range []string{structured.FilePath, structured.File.FilePath} {
			if p != "" && isClaudeMdPath(p) {
				d.addClaudeMdInteraction(p, uuid, ts, "tool_result", seen)
			}
		}
	}

	// Try string form — check for CLAUDE.md path pattern.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.Contains(s, "CLAUDE.md") {
			matches := claudeMdPathRe.FindAllStringSubmatch(s, -1)
			for _, m := range matches {
				d.addClaudeMdInteraction(m[1], uuid, ts, "tool_result", seen)
			}
		}
	}
}

// processFileSnapshot extracts CLAUDE.md paths from file-history-snapshot entries.
func (d *Digest) processFileSnapshot(entry *rawEntry, seen map[string]bool) {
	if len(entry.Snapshot) == 0 {
		return
	}

	var snap struct {
		TrackedFileBackups map[string]json.RawMessage `json:"trackedFileBackups"`
	}
	if err := json.Unmarshal(entry.Snapshot, &snap); err != nil {
		return
	}

	uuid := ""
	if entry.UUID != nil {
		uuid = *entry.UUID
	}
	ts := ""
	if entry.Timestamp != nil {
		ts = *entry.Timestamp
	}

	for filePath := range snap.TrackedFileBackups {
		if isClaudeMdPath(filePath) {
			d.addClaudeMdInteraction(filePath, uuid, ts, "file_snapshot", seen)
		}
	}
}

// isClaudeMdPath returns true if the path refers to a CLAUDE.md file.
func isClaudeMdPath(p string) bool {
	return strings.HasSuffix(p, "CLAUDE.md") || strings.HasSuffix(p, "CLAUDE.md/")
}

func (d *Digest) finalizeAgents(sessionID string, agentSpawns map[string]*AgentInventoryItem, agentResultIDs map[string]bool, agentIDCounts map[string]int) {
	rawDir := store.RawDir(sessionID)

	// Check for agent transcript files in the raw directory.
	agentFiles := make(map[string]bool)
	entries, err := os.ReadDir(rawDir)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, "agent-") && strings.HasSuffix(name, ".jsonl") {
				shortID := strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".jsonl")
				agentFiles[shortID] = true
			}
		}
	}

	// Sort agent IDs for deterministic assignment when multiple candidates exist.
	sortedAgentIDs := make([]string, 0, len(agentIDCounts))
	for id := range agentIDCounts {
		sortedAgentIDs = append(sortedAgentIDs, id)
	}
	sort.Strings(sortedAgentIDs)

	for toolUseID, item := range agentSpawns {
		item.Abandoned = !agentResultIDs[toolUseID]

		// Try to match agent ID counts (entries in main transcript with this agentId).
		// The agentId in entries is a short form, not the tool_use_id. We look for
		// any agentId that matches a known pattern.
		// Iterate in sorted order for deterministic assignment.
		for _, agentID := range sortedAgentIDs {
			count := agentIDCounts[agentID]
			// Heuristic: agent entries reference by a shortened agent ID.
			// We can't perfectly map tool_use_id to agentId without more data,
			// but we collect what we find.
			if item.EntryCount == 0 {
				item.AgentID = agentID
				item.EntryCount = count
				item.HasOwnTranscript = agentFiles[agentID]
			}
		}

		d.Agents.Inventory = append(d.Agents.Inventory, *item)
	}

	d.Agents.Count = len(d.Agents.Inventory)

	// Max depth — for v1, we track if agents exist (depth 1).
	// Deeper nesting would require parsing agent transcript files.
	if d.Agents.Count > 0 {
		d.Agents.MaxDepth = 1
	}
}

// recentToolCall is used for retry anomaly detection.
type recentToolCall struct {
	toolName  string
	uuid      string
	timestamp string
	inputKey  string
}

func detectRetryAnomalies(calls []recentToolCall) []RetryAnomaly {
	if len(calls) < 2 {
		return nil
	}

	var anomalies []RetryAnomaly
	type runKey struct {
		toolName string
		inputKey string
	}

	// Group consecutive calls with same tool+input within the time window.
	i := 0
	for i < len(calls) {
		j := i + 1
		for j < len(calls) {
			if calls[j].toolName != calls[i].toolName || calls[j].inputKey != calls[i].inputKey {
				break
			}

			// Check time window.
			t1, err1 := time.Parse(time.RFC3339, calls[i].timestamp)
			t2, err2 := time.Parse(time.RFC3339, calls[j].timestamp)
			if err1 != nil || err2 != nil {
				break
			}
			if t2.Sub(t1).Seconds() > retryWindowSeconds {
				break
			}
			j++
		}

		if j-i >= 2 {
			uuids := make([]string, 0, j-i)
			for k := i; k < j; k++ {
				uuids = append(uuids, calls[k].uuid)
			}

			windowSec := 0.0
			t1, err1 := time.Parse(time.RFC3339, calls[i].timestamp)
			t2, err2 := time.Parse(time.RFC3339, calls[j-1].timestamp)
			if err1 == nil && err2 == nil {
				windowSec = math.Round(t2.Sub(t1).Seconds()*10) / 10
			}

			anomalies = append(anomalies, RetryAnomaly{
				ToolName:        calls[i].toolName,
				UUIDs:           uuids,
				WindowSeconds:   windowSec,
				InputSimilarity: "exact",
			})
		}

		i = j
	}

	return anomalies
}

// --- Friction detection helpers ---

// fileAccess records a Read/Edit tool call for backtrack detection.
type fileAccess struct {
	filePath string
	toolName string
	uuid     string
	seqIndex int
}

// isSearchTool returns true for tools that perform searches.
func isSearchTool(name string) bool {
	switch name {
	case "Grep", "Glob", "WebSearch":
		return true
	}
	return false
}

// isFileAccessTool returns true for tools that access files by path.
func isFileAccessTool(name string) bool {
	switch name {
	case "Read", "Edit", "Write":
		return true
	}
	return false
}

// isEmptySearchResult checks if a tool_result content represents an empty/no-match result.
// Conservative: returns false if uncertain.
func isEmptySearchResult(content json.RawMessage) bool {
	if len(content) == 0 {
		return true
	}

	// Try string form — most common for search results.
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			return true
		}
		// Common zero-match indicators.
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "no files found") ||
			strings.HasPrefix(lower, "no matches found") ||
			strings.HasPrefix(lower, "no results") ||
			trimmed == "[]" {
			return true
		}
		return false
	}

	// Try array form — empty array means no results.
	var arr []json.RawMessage
	if err := json.Unmarshal(content, &arr); err == nil {
		return len(arr) == 0
	}

	return false
}

// extractFilePath reads file_path or path from a tool_use input JSON.
func extractFilePath(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var fields struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	if fields.FilePath != "" {
		return fields.FilePath
	}
	return fields.Path
}

// detectSearchFumbles finds sequences of 3+ same-tool search calls with different
// inputs within a 60s window — the inverse of retry detection (same tool, different inputs).
func detectSearchFumbles(calls []recentToolCall) []FrictionSignal {
	if len(calls) < 3 {
		return nil
	}

	var signals []FrictionSignal

	i := 0
	for i < len(calls) {
		if !isSearchTool(calls[i].toolName) {
			i++
			continue
		}

		// Collect consecutive same-tool calls within time window.
		j := i + 1
		distinctInputs := map[string]bool{calls[i].inputKey: true}
		for j < len(calls) {
			if calls[j].toolName != calls[i].toolName {
				break
			}
			t1, err1 := time.Parse(time.RFC3339, calls[i].timestamp)
			t2, err2 := time.Parse(time.RFC3339, calls[j].timestamp)
			if err1 != nil || err2 != nil {
				break
			}
			if t2.Sub(t1).Seconds() > retryWindowSeconds {
				break
			}
			distinctInputs[calls[j].inputKey] = true
			j++
		}

		// Emit fumble if 3+ distinct inputs in the window.
		if len(distinctInputs) >= 3 {
			uuids := make([]string, 0, j-i)
			for k := i; k < j; k++ {
				uuids = append(uuids, calls[k].uuid)
			}
			ts := calls[i].timestamp
			signals = append(signals, FrictionSignal{
				Type:      "search_fumble",
				ToolName:  calls[i].toolName,
				UUIDs:     uuids,
				Detail:    fmt.Sprintf("%d distinct search inputs in sequence", len(distinctInputs)),
				Timestamp: ts,
			})
		}

		i = j
	}

	return signals
}

// detectBacktracking finds files accessed 2+ times with significant intervening
// tool calls to different files between accesses.
func detectBacktracking(accesses []fileAccess) []FrictionSignal {
	if len(accesses) < 2 {
		return nil
	}

	// Group accesses by file path.
	groups := make(map[string][]fileAccess)
	for _, a := range accesses {
		groups[a.filePath] = append(groups[a.filePath], a)
	}

	var signals []FrictionSignal
	for filePath, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Check for ≥3 intervening tool calls to different files between any pair.
		for k := 1; k < len(group); k++ {
			prev := group[k-1]
			curr := group[k]

			// Count intervening accesses to different files.
			intervening := 0
			for _, a := range accesses {
				if a.seqIndex > prev.seqIndex && a.seqIndex < curr.seqIndex && a.filePath != filePath {
					intervening++
				}
			}

			if intervening >= 3 {
				signals = append(signals, FrictionSignal{
					Type:     "backtrack",
					ToolName: curr.toolName,
					UUIDs:    []string{prev.uuid, curr.uuid},
					Detail:   fmt.Sprintf("returned to %s after %d intervening file accesses", filepath.Base(filePath), intervening),
				})
				break // One signal per file is enough.
			}
		}
	}

	return signals
}

// WriteDigest writes a Digest to the digests directory as JSON.
func WriteDigest(d *Digest) error {
	dir := filepath.Join(store.Root(), "digests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating digests dir: %w", err)
	}

	path := filepath.Join(dir, d.SessionID+".json")
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling digest: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

// ReadDigest reads a previously written digest from disk.
func ReadDigest(sessionID string) (*Digest, error) {
	path := filepath.Join(store.Root(), "digests", sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var d Digest
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parsing digest: %w", err)
	}
	return &d, nil
}

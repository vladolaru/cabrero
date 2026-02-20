// Package parser provides the pre-parser that converts raw JSONL session
// transcripts into structured digest files for downstream analysis.
package parser

// Digest is the top-level output of the pre-parser.
type Digest struct {
	Version     int    `json:"version"`
	SessionID   string `json:"sessionId"`
	GeneratedAt string `json:"generatedAt"`

	Shape              Shape              `json:"shape"`
	CompactionSegments []CompactionSegment `json:"compactionSegments"`
	Agents             AgentsInfo         `json:"agents"`
	ToolCalls          ToolCalls          `json:"toolCalls"`
	Skills             []SkillEntry       `json:"skills"`
	ClaudeMd           ClaudeMdInfo        `json:"claudeMd"`
	Errors             []ErrorEntry       `json:"errors"`
	Completion         Completion         `json:"completion"`
	TurnDurations      []TurnDuration     `json:"turnDurations"`
	RawUnknown         []RawUnknown       `json:"rawUnknown"`
}

// Shape captures high-level session metrics.
type Shape struct {
	FirstTimestamp  *string    `json:"firstTimestamp"`
	LastTimestamp   *string    `json:"lastTimestamp"`
	DurationSeconds *float64  `json:"durationSeconds"`
	EntryCount      int       `json:"entryCount"`
	TurnCount       int       `json:"turnCount"`
	TokenUsage      TokenUsage `json:"tokenUsage"`
	CacheHitRatio   *float64  `json:"cacheHitRatio"`
	CompactionCount int       `json:"compactionCount"`
	Models          []string  `json:"models"`
	CCVersion       *string   `json:"ccVersion"`
	Cwd             *string   `json:"cwd"` // working directory from first entry
}

// TokenUsage aggregates token consumption across all assistant entries.
type TokenUsage struct {
	TotalInputTokens         int64 `json:"totalInputTokens"`
	TotalOutputTokens        int64 `json:"totalOutputTokens"`
	TotalCacheCreationTokens int64 `json:"totalCacheCreationTokens"`
	TotalCacheReadTokens     int64 `json:"totalCacheReadTokens"`
}

// CompactionSegment describes entries between two compaction boundaries.
type CompactionSegment struct {
	Index            int     `json:"index"`
	SummaryText      string  `json:"summaryText"`
	LeafUUID         string  `json:"leafUuid"`
	EntryCountBefore int     `json:"entryCountBefore"`
	FirstTimestamp   *string `json:"firstTimestamp"`
	LastTimestamp     *string `json:"lastTimestamp"`
}

// AgentsInfo summarises sub-agent usage in the session.
type AgentsInfo struct {
	Count     int                  `json:"count"`
	MaxDepth  int                  `json:"maxDepth"`
	Inventory []AgentInventoryItem `json:"inventory"`
}

// AgentInventoryItem describes a single sub-agent invocation.
type AgentInventoryItem struct {
	AgentID          string `json:"agentId"`
	ParentUUID       string `json:"parentUuid"`
	ToolName         string `json:"toolName"`
	SubagentType     *string `json:"subagentType"`
	Description      *string `json:"description"`
	EntryCount       int    `json:"entryCount"`
	HasOwnTranscript bool   `json:"hasOwnTranscript"`
	Abandoned        bool   `json:"abandoned"`
}

// ToolCalls aggregates tool usage and anomalies.
type ToolCalls struct {
	Summary        map[string]ToolCallDetail `json:"summary"`
	RetryAnomalies []RetryAnomaly            `json:"retryAnomalies"`
}

// ToolCallDetail tracks counts for a specific tool.
type ToolCallDetail struct {
	Count      int    `json:"count"`
	ErrorCount int    `json:"errorCount"`
	FirstUUID  string `json:"firstUuid"`
	LastUUID   string `json:"lastUuid"`
}

// RetryAnomaly flags repeated tool calls with near-identical inputs.
type RetryAnomaly struct {
	ToolName        string   `json:"toolName"`
	UUIDs           []string `json:"uuids"`
	WindowSeconds   float64  `json:"windowSeconds"`
	InputSimilarity string   `json:"inputSimilarity"`
}

// SkillEntry records a Skill tool invocation.
type SkillEntry struct {
	SkillName                    string  `json:"skillName"`
	InvokedAtUUID                string  `json:"invokedAtUuid"`
	Timestamp                    string  `json:"timestamp"`
	ResultUUID                   *string `json:"resultUuid"`
	LoadedBeforeFirstRelevantTool *bool  `json:"loadedBeforeFirstRelevantTool"`
}

// ClaudeMdInfo captures CLAUDE.md presence and interactions in the session.
//
// CLAUDE.md files are always injected into the CC system prompt — their
// influence is passive and constant throughout the session. The "loaded"
// list records which files were active (inferred from the session's cwd).
// The "interactions" list records explicit file operations (Read, Edit, etc.)
// which are a stronger signal of CLAUDE.md being relevant to the task.
type ClaudeMdInfo struct {
	Loaded       []ClaudeMdLoaded      `json:"loaded"`
	Interactions []ClaudeMdInteraction `json:"interactions"`
}

// ClaudeMdLoaded records a CLAUDE.md file inferred as loaded into context.
type ClaudeMdLoaded struct {
	Path   string `json:"path"`   // inferred path (e.g. "~/.claude/CLAUDE.md", "{cwd}/CLAUDE.md")
	Source string `json:"source"` // "user_config" or "project_cwd"
}

// ClaudeMdInteraction records an explicit tool operation on a CLAUDE.md file.
type ClaudeMdInteraction struct {
	Path        string `json:"path"`
	FoundInUUID string `json:"foundInUuid"`
	Timestamp   string `json:"timestamp"`
	Source      string `json:"source"` // "tool_use", "tool_result", "file_snapshot"
}

// ErrorEntry records a tool_result with is_error=true.
type ErrorEntry struct {
	UUID                string  `json:"uuid"`
	ToolUseID           string  `json:"toolUseId"`
	ToolName            *string `json:"toolName"`
	SourceAssistantUUID *string `json:"sourceAssistantUuid"`
	Snippet             string  `json:"snippet"`
	Timestamp           string  `json:"timestamp"`
}

// Completion tracks task management and git activity signals.
type Completion struct {
	TaskCreateCount          int     `json:"taskCreateCount"`
	TaskUpdateCompletedCount int     `json:"taskUpdateCompletedCount"`
	TaskUpdatePendingCount   int     `json:"taskUpdatePendingCount"`
	QueueEnqueueCount        int     `json:"queueEnqueueCount"`
	QueueDequeueCount        int     `json:"queueDequeueCount"`
	GitDiffPresent           bool    `json:"gitDiffPresent"`
	LastToolName             *string `json:"lastToolName"`
}

// TurnDuration records a system turn_duration entry.
type TurnDuration struct {
	UUID       string `json:"uuid"`
	ParentUUID string `json:"parentUuid"`
	DurationMs int64  `json:"durationMs"`
}

// RawUnknown records an entry with an unrecognized type.
type RawUnknown struct {
	Type       string  `json:"type"`
	UUID       *string `json:"uuid"`
	Timestamp  *string `json:"timestamp"`
	LineNumber int     `json:"lineNumber"`
}

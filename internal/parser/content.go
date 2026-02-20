package parser

import (
	"encoding/json"
	"strings"
)

// contentBlock represents a single block in message.content arrays.
type contentBlock struct {
	Type string `json:"type"`

	// text block
	Text string `json:"text,omitempty"`

	// tool_use block
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result block
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string or array
	IsError   bool            `json:"is_error,omitempty"`

	// thinking block
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// parseContentBlocks handles the polymorphic message.content field.
// It can be a JSON string or a JSON array of content blocks.
func parseContentBlocks(raw json.RawMessage) []contentBlock {
	if len(raw) == 0 {
		return nil
	}

	// Try array first (most common in real transcripts).
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
	}

	// Fall back to string — wrap in a single text block.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []contentBlock{{Type: "text", Text: s}}
	}

	return nil
}

// extractToolUseBlocks returns all tool_use content blocks from a list.
func extractToolUseBlocks(blocks []contentBlock) []contentBlock {
	var out []contentBlock
	for _, b := range blocks {
		if b.Type == "tool_use" {
			out = append(out, b)
		}
	}
	return out
}

// extractToolResultBlocks returns all tool_result content blocks from a list.
func extractToolResultBlocks(blocks []contentBlock) []contentBlock {
	var out []contentBlock
	for _, b := range blocks {
		if b.Type == "tool_result" {
			out = append(out, b)
		}
	}
	return out
}

// toolResultSnippet extracts a human-readable snippet from a tool_result content field.
// Returns the first 200 characters.
func toolResultSnippet(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return truncate(s, 200)
	}

	// Try array of content blocks (nested tool results).
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return truncate(strings.Join(parts, " "), 200)
	}

	return truncate(string(raw), 200)
}

// extractTextContent concatenates all text blocks into a single string.
func extractTextContent(blocks []contentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// isOnlyToolResults returns true if all content blocks are tool_result type.
func isOnlyToolResults(blocks []contentBlock) bool {
	if len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if b.Type != "tool_result" {
			return false
		}
	}
	return true
}

// toolUseInputString returns the serialized input JSON with keys sorted,
// suitable for exact-match comparison in retry anomaly detection.
func toolUseInputString(raw json.RawMessage) string {
	// Re-marshal via map to normalize key order.
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return string(raw)
	}
	out, err := json.Marshal(m)
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/retrieval"
)

// Inspect shows a proposal with its full citation chain.
func Inspect(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: cabrero inspect <proposal_id>")
	}

	proposalID := args[0]
	pw, err := pipeline.ReadProposal(proposalID)
	if err != nil {
		return fmt.Errorf("proposal not found: %w", err)
	}

	p := pw.Proposal

	fmt.Printf("Proposal: %s\n", p.ID)
	fmt.Println("──────────────────────────────────────────────────")
	fmt.Printf("Type:       %s\n", p.Type)
	fmt.Printf("Confidence: %s\n", p.Confidence)
	fmt.Printf("Target:     %s\n", p.Target)
	fmt.Printf("Session:    %s\n", pw.SessionID)
	fmt.Println()

	if p.Change != nil {
		fmt.Println("Proposed change:")
		fmt.Println(indent(*p.Change, "  "))
		fmt.Println()
	}

	if p.FlaggedEntry != nil {
		fmt.Println("Flagged entry:")
		fmt.Println(indent(*p.FlaggedEntry, "  "))
		fmt.Println()
	}

	if p.AssessmentSummary != nil {
		fmt.Println("Assessment:")
		fmt.Println(indent(*p.AssessmentSummary, "  "))
		fmt.Println()
	}

	fmt.Println("Rationale:")
	fmt.Println(indent(p.Rationale, "  "))
	fmt.Println()

	if len(p.CitedSkillSignals) > 0 {
		fmt.Printf("Cited skills: %s\n", strings.Join(p.CitedSkillSignals, ", "))
	}
	if len(p.CitedClaudeMdSignals) > 0 {
		fmt.Printf("Cited CLAUDE.md: %s\n", strings.Join(p.CitedClaudeMdSignals, ", "))
	}

	// Citation chain — show the raw transcript entries for each cited UUID.
	if len(p.CitedUUIDs) > 0 {
		fmt.Println()
		fmt.Printf("Citation chain (%d entries):\n", len(p.CitedUUIDs))
		fmt.Println("──────────────────────────────────────────────────")

		for i, uuid := range p.CitedUUIDs {
			raw, err := retrieval.GetEntry(pw.SessionID, uuid)
			if err != nil {
				fmt.Printf("\n[%d] UUID: %s (not found)\n", i+1, uuid)
				continue
			}

			summary := summarizeEntry(raw)
			fmt.Printf("\n[%d] UUID: %s\n", i+1, uuid)
			fmt.Println(indent(summary, "    "))
		}
	}

	return nil
}

// summarizeEntry produces a human-readable summary of a raw JSONL entry.
func summarizeEntry(raw []byte) string {
	var entry struct {
		Type      string  `json:"type"`
		UUID      string  `json:"uuid"`
		Timestamp *string `json:"timestamp"`
		Message   struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
			Model   *string         `json:"model"`
		} `json:"message"`
	}

	if err := json.Unmarshal(raw, &entry); err != nil {
		return "(unparseable entry)"
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Type: %s", entry.Type))

	if entry.Timestamp != nil {
		parts = append(parts, fmt.Sprintf("Time: %s", *entry.Timestamp))
	}

	if entry.Message.Role != "" {
		parts = append(parts, fmt.Sprintf("Role: %s", entry.Message.Role))
	}

	if entry.Message.Model != nil {
		parts = append(parts, fmt.Sprintf("Model: %s", *entry.Message.Model))
	}

	// Extract a text preview from the content.
	if len(entry.Message.Content) > 0 {
		preview := extractContentPreview(entry.Message.Content)
		if preview != "" {
			parts = append(parts, fmt.Sprintf("Content: %s", preview))
		}
	}

	return strings.Join(parts, "\n")
}

func extractContentPreview(raw json.RawMessage) string {
	// Try string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if len(s) > 150 {
			return s[:150] + "…"
		}
		return s
	}

	// Try array of content blocks.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
		Name string `json:"name,omitempty"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			switch b.Type {
			case "text":
				text := b.Text
				if len(text) > 100 {
					text = text[:100] + "…"
				}
				parts = append(parts, text)
			case "tool_use":
				parts = append(parts, fmt.Sprintf("[tool_use: %s]", b.Name))
			case "tool_result":
				parts = append(parts, "[tool_result]")
			case "thinking":
				parts = append(parts, "[thinking]")
			}
		}
		result := strings.Join(parts, " | ")
		if len(result) > 200 {
			return result[:200] + "…"
		}
		return result
	}

	return ""
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

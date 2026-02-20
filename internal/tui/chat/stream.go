package chat

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// revisionFenceRe matches ```revision code fences.
var revisionFenceRe = regexp.MustCompile("(?s)```revision\\s*\\n(.*?)```")

// StartChat spawns a claude CLI subprocess and streams responses.
func StartChat(question string, systemContext string) tea.Cmd {
	return func() tea.Msg {
		// Build the prompt with system context.
		prompt := systemContext + "\n\nUser question: " + question

		cmd := exec.Command("claude",
			"--model", "claude-sonnet-4-6",
			"--print",
			"--output-format", "stream-json",
		)
		cmd.Env = append(cmd.Environ(), "CABRERO_SESSION=1")
		cmd.Stdin = strings.NewReader(prompt)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return message.ChatStreamError{Err: err}
		}

		if err := cmd.Start(); err != nil {
			return message.ChatStreamError{Err: err}
		}

		// Read stdout line by line, parse stream-json format.
		scanner := bufio.NewScanner(stdout)
		var fullResponse strings.Builder

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			// Parse the JSON line.
			var event streamEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				// Unknown format — append raw text.
				fullResponse.WriteString(line)
				continue
			}

			// Extract text content from the event.
			if event.Type == "content_block_delta" && event.Delta.Text != "" {
				fullResponse.WriteString(event.Delta.Text)
			}
		}

		if err := cmd.Wait(); err != nil {
			return message.ChatStreamError{Err: err}
		}

		return message.ChatStreamDone{FullResponse: fullResponse.String()}
	}
}

// streamEvent represents a single JSON event from claude CLI stream-json output.
type streamEvent struct {
	Type  string     `json:"type"`
	Delta eventDelta `json:"delta,omitempty"`
}

type eventDelta struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

// parseRevision extracts the last ```revision``` block from a response.
// Returns nil if no valid revision block is found.
func parseRevision(response string) *string {
	matches := revisionFenceRe.FindAllStringSubmatch(response, -1)
	if len(matches) == 0 {
		return nil
	}

	// Use the last match.
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return nil
	}

	content := strings.TrimSpace(last[1])
	if content == "" {
		return nil
	}

	return &content
}

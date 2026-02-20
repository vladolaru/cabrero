package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// chatTimeout is the maximum wall-clock time for a chat streaming session.
const chatTimeout = 5 * time.Minute

// revisionFenceRe matches ```revision code fences.
var revisionFenceRe = regexp.MustCompile("(?s)```revision\\s*\\n(.*?)```")

// streamMsg is an internal message carrying a token or completion from the stream channel.
type streamMsg struct {
	token string
	done  bool
	full  string
	err   error
}

// StartChat spawns a claude CLI subprocess and streams responses token by token.
// Returns the initial tea.Cmd that begins reading from the stream channel.
func StartChat(question string, systemContext string) tea.Cmd {
	ch := make(chan streamMsg, 64)

	// Start background goroutine to read subprocess output.
	go func() {
		defer close(ch)

		prompt := systemContext + "\n\nUser question: " + question

		ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "claude",
			"--model", "claude-sonnet-4-6",
			"--print",
			"--output-format", "stream-json",
			"--no-session-persistence",
			"--disable-slash-commands",
			"--tools", "",
		)
		cmd.Env = append(cmd.Environ(), "CABRERO_SESSION=1")
		cmd.Stdin = strings.NewReader(prompt)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			ch <- streamMsg{err: err}
			return
		}

		if err := cmd.Start(); err != nil {
			ch <- streamMsg{err: err}
			return
		}

		scanner := bufio.NewScanner(stdout)
		var fullResponse strings.Builder

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var event streamEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				fullResponse.WriteString(line)
				continue
			}

			if event.Type == "content_block_delta" && event.Delta.Text != "" {
				token := event.Delta.Text
				fullResponse.WriteString(token)
				ch <- streamMsg{token: token}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- streamMsg{err: err}
			_ = cmd.Wait()
			return
		}

		if err := cmd.Wait(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				ch <- streamMsg{err: fmt.Errorf("chat timed out after %s", chatTimeout)}
			} else {
				ch <- streamMsg{err: err}
			}
			return
		}

		ch <- streamMsg{done: true, full: fullResponse.String()}
	}()

	// Return a cmd that reads the first message from the channel.
	return waitForStream(ch)
}

// waitForStream returns a tea.Cmd that reads the next message from the stream channel.
func waitForStream(ch <-chan streamMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			// Channel closed unexpectedly.
			return message.ChatStreamDone{FullResponse: ""}
		}
		if msg.err != nil {
			return message.ChatStreamError{Err: msg.err}
		}
		if msg.done {
			return message.ChatStreamDone{FullResponse: msg.full}
		}
		// Token received — emit it and schedule the next read.
		return chatTokenWithContinuation{
			token: msg.token,
			ch:    ch,
		}
	}
}

// chatTokenWithContinuation carries a token and a reference to the channel
// so the model can schedule the next read.
type chatTokenWithContinuation struct {
	token string
	ch    <-chan streamMsg
}

// NextCmd returns the tea.Cmd to read the next token from the stream.
func (t chatTokenWithContinuation) NextCmd() tea.Cmd {
	return waitForStream(t.ch)
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

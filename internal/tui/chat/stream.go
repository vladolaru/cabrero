package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/store"
	"github.com/vladolaru/cabrero/internal/tui/message"
)

// chatTimeout is the maximum wall-clock time for a chat streaming session.
const chatTimeout = 5 * time.Minute

// revisionFenceRe matches ```revision code fences.
var revisionFenceRe = regexp.MustCompile("(?s)```revision\\s*\\n(.*?)```")

// streamMsg is an internal message carrying a token or completion from the stream channel.
type streamMsg struct {
	token    string // text content for final response
	activity string // activity description for muted status area
	newTurn  bool   // true when a new assistant turn starts (discard prior text)
	done     bool
	full     string
	err      error
}

// buildChatArgs constructs the claude CLI argument list for a chat invocation.
// firstMessage=true creates a new session; firstMessage=false resumes it.
func buildChatArgs(cfg ChatConfig, firstMessage bool) []string {
	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-6" // fallback if not set
	}
	args := []string{
		"--model", model,
		"--print",
		"--verbose",
		"--output-format", "stream-json",
		"--disable-slash-commands",
		"--settings", `{"disableAllHooks": true}`, // prevent user hooks from firing
	}

	if firstMessage {
		// Create a new session with full configuration.
		args = append(args, "--session-id", cfg.SessionID)
		args = append(args, "--permission-mode", "dontAsk")
		if cfg.SystemPrompt != "" {
			args = append(args, "--system-prompt", cfg.SystemPrompt)
		}
		if cfg.AllowedTools != "" {
			args = append(args, "--allowedTools", cfg.AllowedTools)
		}
	} else {
		// Resume the existing session.
		args = append(args, "--resume", cfg.SessionID)
	}

	return args
}

// StartChat spawns a claude CLI subprocess and streams responses token by token.
// Uses a persistent CC session: firstMessage=true creates it with --session-id,
// subsequent calls resume it with --resume.
func StartChat(question string, cfg ChatConfig, firstMessage bool) tea.Cmd {
	ch := make(chan streamMsg, 64)

	// Start background goroutine to read subprocess output.
	go func() {
		defer close(ch)

		ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
		defer cancel()

		args := buildChatArgs(cfg, firstMessage)

		debugLog := func(format string, a ...any) {
			if !cfg.Debug {
				return
			}
			chatLog(fmt.Sprintf(format, a...))
		}

		debugLog("session: %s", cfg.SessionID)
		debugLog("args: %v", args)

		cmd := exec.CommandContext(ctx, "claude", args...)
		var stderrBuf bytes.Buffer
		cmd.Env = cleanClaudeEnv()
		cmd.Stdin = strings.NewReader(question)
		cmd.Stderr = &stderrBuf

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			ch <- streamMsg{err: err}
			return
		}

		if err := cmd.Start(); err != nil {
			debugLog("start failed: %v", err)
			ch <- streamMsg{err: err}
			return
		}

		debugLog("subprocess started (pid %d)", cmd.Process.Pid)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB line buffer
		var fullResponse strings.Builder
		var currentMsgID string

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var event streamEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				debugLog("non-json line: %.200s", line)
				fullResponse.WriteString(line)
				continue
			}

			if event.Type == "assistant" {
				// New assistant turn — discard intermediate text from earlier turns.
				if event.Message.ID != "" && event.Message.ID != currentMsgID {
					currentMsgID = event.Message.ID
					fullResponse.Reset()
					ch <- streamMsg{newTurn: true}
				}
				for _, block := range event.Message.Content {
					switch block.Type {
					case "text":
						if block.Text != "" {
							fullResponse.WriteString(block.Text)
							ch <- streamMsg{token: block.Text}
						}
					case "tool_use":
						if block.Name != "" {
							ch <- streamMsg{activity: "Using " + block.Name + "..."}
						}
					case "thinking":
						ch <- streamMsg{activity: "Thinking..."}
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- streamMsg{err: err}
			_ = cmd.Wait()
			return
		}

		if err := cmd.Wait(); err != nil {
			stderr := strings.TrimSpace(stderrBuf.String())
			debugLog("exit error: %v", err)
			if stderr != "" {
				debugLog("stderr: %s", stderr)
			}
			if ctx.Err() == context.DeadlineExceeded {
				ch <- streamMsg{err: fmt.Errorf("chat timed out after %s", chatTimeout)}
			} else if stderr != "" {
				ch <- streamMsg{err: fmt.Errorf("%s", stderr)}
			} else {
				ch <- streamMsg{err: err}
			}
			return
		}

		debugLog("finished, response length: %d bytes", fullResponse.Len())

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
		// Token or activity received — emit and schedule the next read.
		return chatTokenWithContinuation{
			token:    msg.token,
			activity: msg.activity,
			newTurn:  msg.newTurn,
			ch:       ch,
		}
	}
}

// chatTokenWithContinuation carries a token and a reference to the channel
// so the model can schedule the next read.
type chatTokenWithContinuation struct {
	token    string
	activity string
	newTurn  bool
	ch       <-chan streamMsg
}

// NextCmd returns the tea.Cmd to read the next token from the stream.
func (t chatTokenWithContinuation) NextCmd() tea.Cmd {
	return waitForStream(t.ch)
}

// streamEvent represents a single JSON event from claude CLI stream-json output.
// With --verbose, events are conversation-level: "assistant", "user", "system", "result".
// The assistant event contains message.content[] with text/thinking/tool_use blocks.
type streamEvent struct {
	Type    string       `json:"type"`
	Message eventMessage `json:"message,omitempty"`
}

type eventMessage struct {
	ID      string         `json:"id,omitempty"`
	Content []contentBlock `json:"content,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"` // tool name for tool_use blocks
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

// chatLog appends a timestamped line to daemon.log for debug tracing.
func chatLog(msg string) {
	logPath := filepath.Join(store.Root(), "daemon.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[chat] %s\n", msg)
}

// cleanClaudeEnv returns os.Environ() with CLAUDECODE stripped and CABRERO_SESSION=1 added.
// This prevents "cannot be launched inside another Claude Code session" when the TUI
// itself is running inside a Claude Code terminal.
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

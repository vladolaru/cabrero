package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vladolaru/cabrero/internal/store"
)

// notifyHelper caches the resolved path to the cabrero-notify binary.
var (
	notifyHelperOnce sync.Once
	notifyHelperPath string
)

// resolveNotifyHelper finds cabrero-notify next to the cabrero binary.
func resolveNotifyHelper() string {
	notifyHelperOnce.Do(func() {
		path := filepath.Join(store.Root(), "bin", "cabrero-notify")
		if _, err := os.Stat(path); err == nil {
			notifyHelperPath = path
		}
	})
	return notifyHelperPath
}

// Notify sends a macOS notification.
// Prefers the cabrero-notify Swift helper (avoids AppleScript runtime and TCC
// prompts for Desktop/Music/Photos). Falls back to osascript if the helper
// is not installed.
func Notify(title, message string) error {
	if helper := resolveNotifyHelper(); helper != "" {
		return exec.Command(helper, title, message).Run()
	}
	// Fallback: osascript loads the AppleScript runtime and scripting additions
	// (Standard Additions → CoreAudio/AudioToolbox, Digital Hub → media),
	// which can intermittently trigger macOS TCC prompts.
	script := `display notification "` + escapeAppleScript(message) + `" with title "` + escapeAppleScript(title) + `"`
	return exec.Command("osascript", "-e", script).Run()
}

func escapeAppleScript(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

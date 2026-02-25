package logview

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

// LogAppended is returned by FollowTick when new bytes were appended to the log file.
type LogAppended struct {
	NewContent  string
	NewFileSize int64
}

// LogReplaced is returned by FollowTick when the log file shrank (log rotation detected).
// The full new content is included.
type LogReplaced struct {
	Content     string
	NewFileSize int64
}

// FollowTick returns a Bubble Tea command that checks logPath for new content
// since the last read position recorded in m.fileSize.
//
// Returns:
//   - LogAppended if new bytes were written since last read
//   - LogReplaced if the file shrank (log rotation detected)
//   - nil if the file is unchanged, missing, or unreadable
func (m Model) FollowTick(logPath string) tea.Cmd {
	size := m.fileSize
	return func() tea.Msg {
		info, err := os.Stat(logPath)
		if err != nil {
			return nil
		}
		newSize := info.Size()

		if newSize > size {
			// New bytes appended — read only the delta.
			f, err := os.Open(logPath)
			if err != nil {
				return nil
			}
			buf := make([]byte, newSize-size)
			n, _ := f.ReadAt(buf, size)
			f.Close()
			if n > 0 {
				return LogAppended{NewContent: string(buf[:n]), NewFileSize: size + int64(n)}
			}
			return nil
		}

		if newSize < size {
			// File shrank — full reload (log rotation).
			content, _ := os.ReadFile(logPath)
			return LogReplaced{Content: string(content), NewFileSize: int64(len(content))}
		}

		return nil // unchanged
	}
}

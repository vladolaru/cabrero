package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
)

// Config controls daemon timing and logging.
type Config struct {
	PollInterval      time.Duration // how often to check for pending sessions (default 2m)
	StaleInterval     time.Duration // how often to scan for stale sessions (default 30m)
	InterSessionDelay time.Duration // pause between processing sessions (default 30s)
	LogPath           string        // path to daemon log file
	LogMaxSize        int64         // max log file size before rotation (default 5MB)
}

// DefaultConfig returns a Config with production defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval:      2 * time.Minute,
		StaleInterval:     30 * time.Minute,
		InterSessionDelay: 30 * time.Second,
		LogPath:           filepath.Join(store.Root(), "daemon.log"),
		LogMaxSize:        0, // use logger default (5 MB)
	}
}

// Daemon processes sessions automatically in the background.
type Daemon struct {
	config Config
	log    *Logger
}

// New creates a Daemon with the given configuration.
func New(cfg Config) (*Daemon, error) {
	log, err := NewLogger(cfg.LogPath, cfg.LogMaxSize)
	if err != nil {
		return nil, fmt.Errorf("creating logger: %w", err)
	}
	return &Daemon{config: cfg, log: log}, nil
}

// Run starts the daemon loop. It blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	defer d.log.Close()

	// Check for an existing instance.
	if pid, alive := readPID(); alive {
		return fmt.Errorf("another daemon is already running (PID %d)", pid)
	}

	// Write PID file.
	pidPath := pidFilePath()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	defer os.Remove(pidPath)

	d.log.Info("daemon started (PID %d)", os.Getpid())
	d.log.Info("poll=%s stale=%s delay=%s", d.config.PollInterval, d.config.StaleInterval, d.config.InterSessionDelay)

	// Run an immediate scan on startup.
	d.processPending(ctx)

	pollTicker := time.NewTicker(d.config.PollInterval)
	defer pollTicker.Stop()

	staleTicker := time.NewTicker(d.config.StaleInterval)
	defer staleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.log.Info("daemon shutting down")
			return nil
		case <-pollTicker.C:
			d.processPending(ctx)
		case <-staleTicker.C:
			d.scanStale()
		}
	}
}

func (d *Daemon) processPending(ctx context.Context) {
	sessions, err := ScanPending()
	if err != nil {
		d.log.Error("scanning pending sessions: %v", err)
		return
	}
	if len(sessions) == 0 {
		return
	}

	d.log.Info("found %d pending session(s)", len(sessions))

	for i, sid := range sessions {
		// Check for shutdown between sessions.
		select {
		case <-ctx.Done():
			d.log.Info("shutdown requested, stopping after %d/%d sessions", i, len(sessions))
			return
		default:
		}

		d.processOne(sid)

		// Rate limit between sessions (skip delay after the last one).
		if i < len(sessions)-1 && d.config.InterSessionDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d.config.InterSessionDelay):
			}
		}
	}
}

func (d *Daemon) processOne(sessionID string) {
	d.log.Info("processing session %s", sessionID)

	result, err := pipeline.Run(sessionID, false)
	if err != nil {
		d.log.Error("pipeline failed for %s: %v", sessionID, err)
		d.markError(sessionID)
		return
	}

	proposalCount := 0
	if result.SonnetOutput != nil {
		proposalCount = len(result.SonnetOutput.Proposals)
	}

	d.log.Info("processed %s: %d proposals", sessionID, proposalCount)

	if proposalCount > 0 {
		msg := fmt.Sprintf("%d new proposal(s) from session %s", proposalCount, shortID(sessionID))
		if err := Notify("Cabrero", msg); err != nil {
			d.log.Error("notification failed: %v", err)
		}
	}
}

func (d *Daemon) markError(sessionID string) {
	meta, err := store.ReadMetadata(sessionID)
	if err != nil {
		d.log.Error("reading metadata for %s to mark error: %v", sessionID, err)
		return
	}
	meta.Status = "error"
	if err := store.WriteMetadata(store.RawDir(sessionID), meta); err != nil {
		d.log.Error("writing error status for %s: %v", sessionID, err)
	}
}

func (d *Daemon) scanStale() {
	recovered, err := ScanStale(d.log)
	if err != nil {
		d.log.Error("stale scan: %v", err)
		return
	}
	if recovered > 0 {
		d.log.Info("stale scan: recovered %d session(s)", recovered)
	}
}

// --- PID file helpers ---

func pidFilePath() string {
	return filepath.Join(store.Root(), "daemon.pid")
}

// readPID reads the PID file and checks if the process is still alive.
// Returns (pid, true) if alive, (0, false) otherwise.
func readPID() (int, bool) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		return 0, false
	}
	// Signal 0 checks if process exists without actually sending a signal.
	if err := syscall.Kill(pid, 0); err != nil {
		return 0, false
	}
	return pid, true
}

// IsDaemonRunning reports whether a daemon process is alive, along with its PID.
func IsDaemonRunning() (int, bool) {
	return readPID()
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

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
	PollInterval      time.Duration           // how often to check for pending sessions (default 2m)
	StaleInterval     time.Duration           // how often to scan for stale sessions (default 30m)
	InterSessionDelay time.Duration           // pause between processing sessions (default 30s)
	LogPath           string                  // path to daemon log file
	LogMaxSize        int64                   // max log file size before rotation (default 5MB)
	Pipeline          pipeline.PipelineConfig // LLM invocation parameters
}

// DefaultConfig returns a Config with production defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval:      2 * time.Minute,
		StaleInterval:     30 * time.Minute,
		InterSessionDelay: 30 * time.Second,
		LogPath:           filepath.Join(store.Root(), "daemon.log"),
		LogMaxSize:        0, // use logger default (5 MB)
		Pipeline:          pipeline.DefaultPipelineConfig(),
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
	d.processQueued(ctx)

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
			d.processQueued(ctx)
		case <-staleTicker.C:
			d.scanStale()
		}
	}
}

func (d *Daemon) processQueued(ctx context.Context) {
	queued, err := ScanQueued()
	if err != nil {
		d.log.Error("scanning queued sessions: %v", err)
		return
	}
	if len(queued) == 0 {
		return
	}

	d.log.Info("found %d queued session(s)", len(queued))

	// Group by project for smart batching.
	byProject := make(map[string][]QueuedSession)
	var projectOrder []string
	for _, p := range queued {
		key := p.Project
		if _, seen := byProject[key]; !seen {
			projectOrder = append(projectOrder, key)
		}
		byProject[key] = append(byProject[key], p)
	}

	first := true
	for _, project := range projectOrder {
		sessions := byProject[project]

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Rate limit between groups (skip delay before the first one).
		if !first && d.config.InterSessionDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d.config.InterSessionDelay):
			}
		}
		first = false

		if project == "" || len(sessions) == 1 {
			// No project metadata or solo session — process individually.
			for i, s := range sessions {
				select {
				case <-ctx.Done():
					return
				default:
				}

				d.processOne(s.SessionID)

				// Rate limit between sessions within the group.
				if i < len(sessions)-1 && d.config.InterSessionDelay > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(d.config.InterSessionDelay):
					}
				}
			}
			continue
		}

		d.processProjectBatch(ctx, project, sessions)
	}

	// Notify when queue is fully drained.
	if err := Notify("Cabrero", "Queue processing complete"); err != nil {
		d.log.Error("queue-drain notification failed: %v", err)
	}
}

// processProjectBatch runs the Classifier individually on each session in a project,
// then batches sessions flagged as "evaluate" into a single Evaluator invocation.
func (d *Daemon) processProjectBatch(ctx context.Context, project string, sessions []QueuedSession) {
	d.log.Info("batch: %d session(s) for project %s", len(sessions), store.ProjectDisplayName(project))

	batchSessions := make([]pipeline.BatchSession, len(sessions))
	for i, s := range sessions {
		batchSessions[i] = pipeline.BatchSession{SessionID: s.SessionID}
	}

	bp := &pipeline.BatchProcessor{
		Config: d.config.Pipeline,
		OnStatus: func(sessionID string, event pipeline.BatchEvent) {
			switch event.Type {
			case "classifier_done":
				if event.Triage == "clean" {
					d.log.Info("session %s triaged as clean", shortID(sessionID))
				}
			case "error":
				d.log.Error("pipeline error for %s: %v", shortID(sessionID), event.Error)
			}
		},
	}

	results := bp.ProcessGroup(ctx, batchSessions)

	totalProposals := 0
	toEvalCount := 0
	for _, r := range results {
		totalProposals += r.Proposals
		if r.Triage == "evaluate" {
			toEvalCount++
		}
	}

	d.log.Info("batch: %d of %d session(s) needed evaluation, %d proposals",
		toEvalCount, len(sessions), totalProposals)

	if totalProposals > 0 {
		msg := fmt.Sprintf("%d new proposal(s) from %d session(s)", totalProposals, len(sessions))
		if err := Notify("Cabrero", msg); err != nil {
			d.log.Error("notification failed: %v", err)
		}
	}
}

func (d *Daemon) processOne(sessionID string) {
	d.log.Info("processing session %s", sessionID)

	result, err := pipeline.Run(sessionID, false, d.config.Pipeline)
	if err != nil {
		d.log.Error("pipeline failed for %s: %v", sessionID, err)
		if markErr := store.MarkError(sessionID); markErr != nil {
			d.log.Error("marking error for %s: %v", sessionID, markErr)
		}
		return
	}

	proposalCount := 0
	if result.EvaluatorOutput != nil {
		proposalCount = len(result.EvaluatorOutput.Proposals)
	}

	d.log.Info("processed %s: %d proposals", sessionID, proposalCount)

	if proposalCount > 0 {
		msg := fmt.Sprintf("%d new proposal(s) from session %s", proposalCount, shortID(sessionID))
		if err := Notify("Cabrero", msg); err != nil {
			d.log.Error("notification failed: %v", err)
		}
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

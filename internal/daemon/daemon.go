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
	pending, err := ScanPending()
	if err != nil {
		d.log.Error("scanning pending sessions: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	d.log.Info("found %d pending session(s)", len(pending))

	// Group by project for smart batching.
	byProject := make(map[string][]PendingSession)
	var projectOrder []string
	for _, p := range pending {
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
}

// processProjectBatch runs Haiku individually on each session in a project,
// then batches sessions flagged as "evaluate" into a single Sonnet invocation.
func (d *Daemon) processProjectBatch(ctx context.Context, project string, sessions []PendingSession) {
	d.log.Info("batch: %d session(s) for project %s", len(sessions), store.ProjectDisplayName(project))

	// Phase 1: Run Haiku individually on each session.
	var toEvaluate []pipeline.BatchSession
	for _, s := range sessions {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, err := pipeline.RunThroughHaiku(s.SessionID, d.config.Pipeline)
		if err != nil {
			d.log.Error("haiku failed for %s: %v", s.SessionID, err)
			d.markError(s.SessionID)
			continue
		}

		if result.HaikuOutput.Triage == "clean" {
			d.log.Info("session %s triaged as clean", shortID(s.SessionID))
			d.markProcessed(s.SessionID)
			continue
		}

		toEvaluate = append(toEvaluate, pipeline.BatchSession{
			SessionID:   s.SessionID,
			Digest:      result.Digest,
			HaikuOutput: result.HaikuOutput,
		})
	}

	if len(toEvaluate) == 0 {
		return
	}

	// Phase 2: Run Sonnet — batch if 2+, individual if 1.
	if len(toEvaluate) == 1 {
		s := toEvaluate[0]
		d.runSonnetSingle(s)
	} else {
		d.runSonnetBatch(toEvaluate)
	}
}

func (d *Daemon) runSonnetSingle(s pipeline.BatchSession) {
	d.log.Info("running Sonnet on session %s", shortID(s.SessionID))
	sonnetOutput, err := pipeline.RunSonnet(s.SessionID, s.Digest, s.HaikuOutput, d.config.Pipeline)
	if err != nil {
		d.log.Error("sonnet failed for %s: %v", s.SessionID, err)
		d.markError(s.SessionID)
		return
	}

	d.persistSonnetOutput(s.SessionID, sonnetOutput)
}

func (d *Daemon) runSonnetBatch(sessions []pipeline.BatchSession) {
	d.log.Info("running batched Sonnet on %d sessions", len(sessions))
	sonnetOutput, err := pipeline.RunSonnetBatch(sessions, d.config.Pipeline)
	if err != nil {
		d.log.Error("sonnet batch failed: %v", err)
		for _, s := range sessions {
			d.markError(s.SessionID)
		}
		return
	}

	// Persist output and proposals for each session in the batch.
	for _, s := range sessions {
		d.persistSonnetOutput(s.SessionID, sonnetOutput)
	}
}

func (d *Daemon) persistSonnetOutput(sessionID string, output *pipeline.SonnetOutput) {
	if err := pipeline.WriteSonnetOutput(sessionID, output); err != nil {
		d.log.Error("writing sonnet output for %s: %v", sessionID, err)
	}

	proposalCount := 0
	for i := range output.Proposals {
		p := &output.Proposals[i]
		if err := pipeline.WriteProposal(p, sessionID); err != nil {
			d.log.Error("writing proposal %s: %v", p.ID, err)
		}
		proposalCount++
	}

	d.markProcessed(sessionID)

	if proposalCount > 0 {
		msg := fmt.Sprintf("%d new proposal(s) from session %s", proposalCount, shortID(sessionID))
		if err := Notify("Cabrero", msg); err != nil {
			d.log.Error("notification failed: %v", err)
		}
	}
}

func (d *Daemon) markProcessed(sessionID string) {
	meta, err := store.ReadMetadata(sessionID)
	if err != nil {
		d.log.Error("reading metadata for %s to mark processed: %v", sessionID, err)
		return
	}
	meta.Status = "processed"
	if err := store.WriteMetadata(store.RawDir(sessionID), meta); err != nil {
		d.log.Error("writing processed status for %s: %v", sessionID, err)
	}
}

func (d *Daemon) processOne(sessionID string) {
	d.log.Info("processing session %s", sessionID)

	result, err := pipeline.Run(sessionID, false, d.config.Pipeline)
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

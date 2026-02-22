package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vladolaru/cabrero/internal/daemon"
)

// Daemon starts the background session processor.
func Daemon(args []string) error {
	cfg := daemon.DefaultConfig()
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	poll := fs.Duration("poll", cfg.PollInterval, "how often to check for pending sessions")
	stale := fs.Duration("stale", cfg.StaleInterval, "how often to scan for stale sessions")
	delay := fs.Duration("delay", cfg.InterSessionDelay, "pause between processing sessions")
	debug := fs.Bool("debug", false, "persist CC sessions for classifier/evaluator inspection")
	classifierMaxTurns := fs.Int("classifier-max-turns", cfg.Pipeline.ClassifierMaxTurns, "max agentic turns for Classifier")
	evaluatorMaxTurns := fs.Int("evaluator-max-turns", cfg.Pipeline.EvaluatorMaxTurns, "max agentic turns for Evaluator")
	classifierTimeout := fs.Duration("classifier-timeout", cfg.Pipeline.ClassifierTimeout, "timeout for Classifier")
	evaluatorTimeout := fs.Duration("evaluator-timeout", cfg.Pipeline.EvaluatorTimeout, "timeout for Evaluator")
	classifierModel := fs.String("classifier-model", cfg.Pipeline.ClassifierModel, "Claude model for Classifier")
	evaluatorModel := fs.String("evaluator-model", cfg.Pipeline.EvaluatorModel, "Claude model for Evaluator")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg.PollInterval = *poll
	cfg.StaleInterval = *stale
	cfg.InterSessionDelay = *delay
	cfg.Pipeline.Debug = *debug
	cfg.Pipeline.ClassifierMaxTurns = *classifierMaxTurns
	cfg.Pipeline.EvaluatorMaxTurns = *evaluatorMaxTurns
	cfg.Pipeline.ClassifierTimeout = *classifierTimeout
	cfg.Pipeline.EvaluatorTimeout = *evaluatorTimeout
	cfg.Pipeline.ClassifierModel = *classifierModel
	cfg.Pipeline.EvaluatorModel = *evaluatorModel

	d, err := daemon.New(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\nReceived %s, shutting down...\n", sig)
		cancel()
	}()

	return d.Run(ctx)
}

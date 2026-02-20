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
	haikuMaxTurns := fs.Int("haiku-max-turns", cfg.Pipeline.HaikuMaxTurns, "max agentic turns for Haiku classifier")
	sonnetMaxTurns := fs.Int("sonnet-max-turns", cfg.Pipeline.SonnetMaxTurns, "max agentic turns for Sonnet evaluator")
	haikuTimeout := fs.Duration("haiku-timeout", cfg.Pipeline.HaikuTimeout, "timeout for Haiku classifier")
	sonnetTimeout := fs.Duration("sonnet-timeout", cfg.Pipeline.SonnetTimeout, "timeout for Sonnet evaluator")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg.PollInterval = *poll
	cfg.StaleInterval = *stale
	cfg.InterSessionDelay = *delay
	cfg.Pipeline.HaikuMaxTurns = *haikuMaxTurns
	cfg.Pipeline.SonnetMaxTurns = *sonnetMaxTurns
	cfg.Pipeline.HaikuTimeout = *haikuTimeout
	cfg.Pipeline.SonnetTimeout = *sonnetTimeout

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

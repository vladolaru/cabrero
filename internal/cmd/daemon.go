package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vladolaru/cabrero/internal/daemon"
)

// Daemon starts the background session processor.
func Daemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	poll := fs.Duration("poll", 2*time.Minute, "how often to check for pending sessions")
	stale := fs.Duration("stale", 30*time.Minute, "how often to scan for stale sessions")
	delay := fs.Duration("delay", 30*time.Second, "pause between processing sessions")
	haikuMaxTurns := fs.Int("haiku-max-turns", 15, "max agentic turns for Haiku classifier")
	sonnetMaxTurns := fs.Int("sonnet-max-turns", 20, "max agentic turns for Sonnet evaluator")
	haikuTimeout := fs.Duration("haiku-timeout", 2*time.Minute, "timeout for Haiku classifier")
	sonnetTimeout := fs.Duration("sonnet-timeout", 5*time.Minute, "timeout for Sonnet evaluator")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := daemon.DefaultConfig()
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

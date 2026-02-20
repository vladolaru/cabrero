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
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := daemon.DefaultConfig()
	cfg.PollInterval = *poll
	cfg.StaleInterval = *stale
	cfg.InterSessionDelay = *delay

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

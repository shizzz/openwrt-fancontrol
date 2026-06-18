// ===== file: cmd/fancontrol/main.go =====

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/openwr-fancontrol/internal/config"
	"github.com/openwr-fancontrol/internal/control"
	"github.com/openwr-fancontrol/internal/log"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Errorf("failed to load config: %v", err)
		os.Exit(1)
	}

	logger := log.New(cfg.Debug)
	logger.Infof("openwr-fancontrol starting (mode=%s dry_run=%v)", cfg.ControlMode, cfg.DryRun)

	loop := control.NewLoop(cfg, logger)

	// procd sends SIGTERM on stop; handle gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		logger.Infof("received signal %s, shutting down", sig)
		loop.Stop()
	}()

	if err := loop.Run(); err != nil {
		logger.Errorf("control loop exited with error: %v", err)
		os.Exit(1)
	}

	logger.Infof("openwr-fancontrol stopped cleanly")
}
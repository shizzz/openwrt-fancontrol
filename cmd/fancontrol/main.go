// ===== file: cmd/fancontrol/main.go =====

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/shizzz/openwrt-fancontrol/internal/config"
	"github.com/shizzz/openwrt-fancontrol/internal/control"
	"github.com/shizzz/openwrt-fancontrol/internal/log"
)

// Set at link time via -ldflags "-X main.Version=... -X main.Commit=... -X main.BuildDate=...".
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	showVersion := flag.Bool("v", false, "print version and exit")
	flag.BoolVar(showVersion, "version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		printVersion()
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Errorf("failed to load config: %v", err)
		os.Exit(1)
	}

	if len(cfg.Fans) == 0 {
		log.Errorf("no fans enabled; nothing to do")
		os.Exit(1)
	}

	logger := log.New(false)
	logger.Infof("openwrt-fancontrol starting with %d fan(s)", len(cfg.Fans))
	for _, f := range cfg.Fans {
		logger.Infof("  - %s mode=%s setpoint=%.1f°C pwm=%s sensor=%s",
			f.Name, f.Mode, f.Setpoint, f.PWMPath, f.ThermalPath)
	}

	loops := make([]*control.Loop, len(cfg.Fans))
	for i := range cfg.Fans {
		loops[i] = control.NewLoop(&cfg.Fans[i], logger)
	}

	// procd sends SIGTERM on stop; handle gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		logger.Infof("received signal %s, shutting down", sig)
		for _, l := range loops {
			l.Stop()
		}
	}()

	var wg sync.WaitGroup
	for _, l := range loops {
		wg.Add(1)
		go func(l *control.Loop) {
			defer wg.Done()
			if err := l.Run(); err != nil {
				logger.Errorf("loop %q exited with error: %v", l.Name(), err)
			}
		}(l)
	}
	wg.Wait()

	logger.Infof("openwrt-fancontrol stopped cleanly")
}

func printVersion() {
	fmt.Printf("openwrt-fancontrol %s", Version)
	if Commit != "" && Commit != "unknown" {
		fmt.Printf(" (%s)", Commit)
	}
	if BuildDate != "" && BuildDate != "unknown" {
		fmt.Printf(" built %s", BuildDate)
	}
	fmt.Println()
}
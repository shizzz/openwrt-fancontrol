// ===== file: cmd/fancontrol/main.go =====

package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/shizzz/openwrt-fancontrol/internal/config"
	"github.com/shizzz/openwrt-fancontrol/internal/control"
	"github.com/shizzz/openwrt-fancontrol/internal/log"
)

func main() {
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
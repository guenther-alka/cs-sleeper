package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/replcheck"
	"github.com/guenther-alka/cs-sleeper/internal/sleeper"
	"github.com/guenther-alka/cs-sleeper/internal/sysio"
)

// daemonCmd runs the idle-detection loop.
func daemonCmd(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	configPath := fs.String("config", "", "config file (default /opt/csweb-gui/_cfg/cs-sleeper)")
	foreground := fs.Bool("foreground", false, "stay in foreground and log to stdout")
	once := fs.Bool("once", false, "run a single sample and exit (diagnostics)")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if !cfg.Enabled {
		fmt.Fprintln(os.Stderr, "cs-sleeper disabled in config (enabled = no); nothing to do")
		os.Exit(0)
	}

	logger := setupLogger(cfg, *foreground)
	if err := acquireLock(cfg.PidFile); err != nil {
		logger.Printf("FATAL: %v", err)
		os.Exit(1)
	}
	defer os.Remove(cfg.PidFile)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		logger.Printf("received signal, stopping")
		os.Remove(cfg.PidFile)
		os.Exit(0)
	}()

	devices := managedDevices(cfg)
	logger.Printf("cs-sleeper %s starting: %d device(s) managed: %s",
		version, len(devices), strings.Join(devices, ","))
	if len(devices) == 0 {
		logger.Printf("no devices configured (hd = ...); exiting")
		os.Exit(0)
	}

	// Set the drive-internal standby timer as a fallback, so disks still
	// spin down even if the sleeper is not running.
	for _, d := range devices {
		if out, err := sleeper.SetStandbyTimer(d, cfg.StandbyMin); err != nil {
			logger.Printf("warning: cannot set standby timer on %s: %v (%s)", d, err, strings.TrimSpace(out))
		}
	}

	opt := sleeper.Options{
		Wait:       time.Duration(cfg.Wait) * time.Second,
		VerifyIdle: cfg.VerifyIdle,
		AllowSleep: func(t time.Time) bool { return !cfg.InWindow(t.Hour()) },
	}
	engine := sleeper.NewEngine(devices, opt)

	reader := sysio.NewReader()
	interval := time.Duration(cfg.Interval) * time.Second
	if interval <= 0 {
		interval = time.Second
	}

	prev := sample(reader, cfg.Interval)
	for {
		start := time.Now()
		cur := sample(reader, cfg.Interval)
		now := time.Now()
		rep := replcheck.Check()

		for _, d := range engine.Update(activeSet(prev, cur, devices), now) {
			if rep.Active {
				logger.Printf("refusing to sleep %s: zfs send/receive in flight", d)
				continue
			}
			out, err := sleeper.Sleep(d)
			if err != nil {
				logger.Printf("sleep %s failed: %v (%s)", d, err, strings.TrimSpace(out))
			} else {
				logger.Printf("sleep %s -> standby (%s)", d, strings.TrimSpace(out))
			}
		}

		writeState(cfg, engine, now, rep, logger)

		if *once {
			return
		}
		prev = cur
		if elapsed := time.Since(start); elapsed < interval {
			time.Sleep(interval - elapsed)
		}
	}
}

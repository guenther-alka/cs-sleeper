package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/guenther-alka/cs-sleeper/internal/sysio"
)

// setupLogger returns a logger writing to the configured file (or stdout in
// foreground mode), falling back to stderr if the log file cannot be opened.
func setupLogger(cfg *Config, foreground bool) *log.Logger {
	if foreground {
		return log.New(os.Stdout, "", log.LstdFlags)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755); err == nil {
		if f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			return log.New(f, "", log.LstdFlags)
		}
	}
	return log.New(os.Stderr, "", log.LstdFlags)
}

// acquireLock ensures only one daemon runs, using a pid-file.
func acquireLock(pidFile string) error {
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		return fmt.Errorf("cannot create pid dir: %w", err)
	}
	if data, err := os.ReadFile(pidFile); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 && processAlive(pid) {
			return fmt.Errorf("cs-sleeper already running (pid %d, pid-file %s)", pid, pidFile)
		}
	}
	return os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// managedDevices returns the normalized list of devices to manage.
func managedDevices(cfg *Config) []string {
	excl := make(map[string]bool, len(cfg.Exclude))
	for _, e := range cfg.Exclude {
		excl[sysio.Normalize(e)] = true
	}
	var out []string
	for _, d := range cfg.HD {
		n := sysio.Normalize(d)
		if n != "" && !excl[n] {
			out = append(out, n)
		}
	}
	return out
}

// sample reads one sample and returns counters keyed by normalized device name.
func sample(r sysio.Reader, window int) map[string]sysio.Counter {
	out := map[string]sysio.Counter{}
	cs, err := r.Sample(window)
	if err != nil {
		return out
	}
	for _, c := range cs {
		out[sysio.Normalize(c.Device)] = c
	}
	return out
}

// activeSet marks each managed device active if the current sample shows I/O
// since the previous sample.
func activeSet(prev, cur map[string]sysio.Counter, devices []string) map[string]bool {
	out := make(map[string]bool, len(devices))
	for _, d := range devices {
		c, ok := cur[d]
		if !ok {
			out[d] = false
			continue
		}
		out[d] = sysio.Active(prev[d], c)
	}
	return out
}

package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/replcheck"
	"github.com/guenther-alka/cs-sleeper/internal/sleeper"
)

// diskState is one managed disk's state as persisted for `status`.
type diskState struct {
	Device      string    `json:"device"`
	Sleeping    bool      `json:"sleeping"`
	LastActive  time.Time `json:"last_active"`
	LastWake    time.Time `json:"last_wake,omitempty"`
	IdleSeconds int       `json:"idle_seconds"`
}

// stateJSON is the daemon state file written to state-dir/state.json.
type stateJSON struct {
	Version    string      `json:"version"`
	PID        int         `json:"pid"`
	Updated    time.Time   `json:"updated"`
	ReplActive bool        `json:"repl_active"`
	Disks      []diskState `json:"disks"`
}

func writeState(cfg *Config, engine *sleeper.Engine, now time.Time, rep replcheck.Activity, logger *log.Logger) {
	snap := engine.Snapshot()
	disks := make([]diskState, 0, len(snap))
	for _, d := range snap {
		idle := 0
		if !d.IdleSince.IsZero() {
			idle = int(now.Sub(d.IdleSince).Seconds())
		}
		disks = append(disks, diskState{
			Device:      d.Device,
			Sleeping:    d.Sleeping,
			LastActive:  d.LastActive,
			LastWake:    d.LastWake,
			IdleSeconds: idle,
		})
	}
	st := stateJSON{Version: version, PID: os.Getpid(), Updated: now, ReplActive: rep.Active, Disks: disks}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		logger.Printf("warning: cannot create state dir: %v", err)
		return
	}
	if err := os.WriteFile(filepath.Join(cfg.StateDir, "state.json"), b, 0o644); err != nil {
		logger.Printf("warning: cannot write state file: %v", err)
	}
}

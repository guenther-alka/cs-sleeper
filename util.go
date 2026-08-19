package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/sysio"
	"github.com/guenther-alka/cs-sleeper/internal/zfs"
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

// neverSleepSet returns the normalized set of devices that must never be
// slept or exported: the configured `exclude` list, the OS boot disk(s), and
// the boot pool's member disks. This is the single source of truth for the
// "never-sleep" guarantee documented in README.md; every command that can
// put a disk to standby or export a pool -- not just the continuous daemon
// loop -- must consult it (see guardDevice/guardBootPool below).
func neverSleepSet(cfg *Config) map[string]bool {
	excl := make(map[string]bool, len(cfg.Exclude))
	for _, e := range cfg.Exclude {
		excl[sysio.Normalize(e)] = true
	}
	for _, d := range sysio.BootDisks() {
		excl[d] = true
	}
	if bp, err := zfs.BootPool(); err == nil && bp != "" {
		for _, d := range zfs.DisksOfPoolSafe(bp) {
			excl[d] = true
		}
	}
	return excl
}

// guardDevice refuses to act on device if it is in the never-sleep set
// (exclude list, OS boot disk, or a boot-pool member disk).
func guardDevice(cfg *Config, device string) error {
	if neverSleepSet(cfg)[sysio.Normalize(device)] {
		return fmt.Errorf("refusing to act on %s: protected (boot disk, boot pool member, or in exclude list)", device)
	}
	return nil
}

// guardBootPool refuses an action on pool if it is the boot pool: exporting
// it, or standby-ing its disks, would take the running OS down with it.
func guardBootPool(pool string) error {
	if bp, err := zfs.BootPool(); err == nil && bp != "" && bp == pool {
		return fmt.Errorf("refusing to act on pool %q: it is the boot pool", pool)
	}
	return nil
}

// excludeDevices returns list with every device present in excl removed.
func excludeDevices(list []string, excl map[string]bool) []string {
	if len(excl) == 0 {
		return list
	}
	out := make([]string, 0, len(list))
	for _, d := range list {
		if !excl[d] {
			out = append(out, d)
		}
	}
	return out
}

// managedDevices returns the normalized list of devices to manage: the union
// of the free disks listed in `disks` and the member disks of the configured
// `pools` (resolved via zpool status), minus the never-sleep set.
func managedDevices(cfg *Config) []string {
	excl := neverSleepSet(cfg)
	seen := make(map[string]bool)
	var out []string
	add := func(name string) {
		n := sysio.Normalize(name)
		if n != "" && !excl[n] && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, d := range cfg.Disks {
		add(d)
	}
	for _, d := range zfs.DisksOfPools(cfg.Pools) {
		add(d)
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

// reverifyIdle re-samples disk I/O after a pool flush and returns the subset of
// disks that stayed idle. A write that landed in the ZFS RAM write cache during
// the flush (or right after it) will hit the disks on the next transaction-group
// commit (~5 s by default), so sampling again avoids spinning down a disk that
// would wake immediately. Disks that cannot be sampled are kept (fail-open: a
// broken reader must not block the sleep).
func reverifyIdle(disks []string, window time.Duration) []string {
	secs := int(window / time.Second)
	if secs < 1 {
		secs = 1
	}
	r := sysio.NewReader()
	prev := sample(r, secs)
	time.Sleep(window)
	cur := sample(r, secs)
	idle := make([]string, 0, len(disks))
	for _, d := range disks {
		if sysio.Active(prev[d], cur[d]) || prev[d].Active {
			continue
		}
		idle = append(idle, d)
	}
	return idle
}

// sameStringSet reports whether a and b contain the same set of strings.
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]bool, len(a))
	for _, s := range a {
		m[s] = true
	}
	for _, s := range b {
		if !m[s] {
			return false
		}
	}
	return true
}

// writeJSONAtomic writes v as indented JSON to path via a temp file + rename.
func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

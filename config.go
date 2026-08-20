package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Window is a daily hour range; End may be < Start to cross midnight.
type Window struct {
	Start, End int
}

// Active reports whether hour h (0-23) falls inside the window.
func (w Window) Active(h int) bool {
	if w.Start <= w.End {
		return h >= w.Start && h < w.End
	}
	return h >= w.Start || h < w.End
}

// Config is the parsed cs-sleeper configuration.
type Config struct {
	Enabled    bool
	Disks      []string // free/standalone disks to manage directly
	Exclude    []string // disks never to touch
	Pools      []string // pools whose member disks are managed (via zpool status)
	Activity   []Window
	Wait       int
	Interval   int
	Policy     string
	StandbyMin int
	Wake       string
	Parallel   int
	VerifyIdle int
	VMMode     string // off | proxmox_suspend | proxmox_shutdown
	PoolRescan int    // seconds between pool disk re-resolution (daemon)
	StateDir   string
	LogFile    string
	LogLevel   string
	PidFile    string

	// ExportPools are pools that follow a forced export/import schedule
	// (ExportTimetable) instead of -- on top of -- ordinary idle-based
	// sleep: outside every window the pool is exported (zpool export, like
	// `sleeppool --export`), inside a window it is imported and woken
	// (like `wakepool`). Every pool listed here must also appear in Pools.
	// Both ExportPools and ExportTimetable must be non-empty for this to
	// do anything -- listing a pool with no timetable configured is
	// treated as the feature being off for it, not "always exported".
	ExportPools     []string
	ExportTimetable []MinWindow

	// PoolWindow gives individual pools their own allow-sleep window,
	// replacing the global Activity window for that pool's disks (a pool
	// not present here keeps using Activity, same as before this existed).
	// Every pool key must also appear in Pools. Free disks (Disks, not
	// part of any named pool) always use Activity -- they have no pool to
	// key a per-pool window by.
	PoolWindow map[string][]MinWindow

	// ActiveTimetable is the HH:MM-granularity successor to Activity
	// (which only has hour granularity): the global allow-sleep window
	// applied to every disk that has no PoolWindow override -- exactly
	// the same role Activity plays, just finer-grained. When non-empty
	// it takes priority over Activity; Activity itself is kept only as
	// a legacy fallback for hand-edited config files that still use the
	// old hour-only syntax (see SleepAllowed below), and is no longer
	// written by csweb-gui's own Settings form as of the ActiveTimetable
	// UI round.
	ActiveTimetable []MinWindow
}

// InWindow reports whether the given hour is inside any activity window.
func (c *Config) InWindow(h int) bool {
	for _, w := range c.Activity {
		if w.Active(h) {
			return true
		}
	}
	return false
}

// SleepAllowed reports whether sleep is allowed for the global (non-
// PoolWindow-overridden) case at time t: ActiveTimetable, when set, takes
// priority over the legacy hour-granularity Activity field entirely (not
// merged with it) -- a config using the new HH:MM field is fully switched
// over, not layering two independent timetables on top of each other.
func (c *Config) SleepAllowed(t time.Time) bool {
	if len(c.ActiveTimetable) > 0 {
		return !inAnyMinWindow(c.ActiveTimetable, minOfDay(t))
	}
	return !c.InWindow(t.Hour())
}

// MinWindow is a daily minute-of-day range (0-1439); End may be < Start to
// cross midnight, same convention as Window (which is hour-granularity).
// Used by ExportTimetable and PoolWindow, which need finer-than-hour
// control than the original Activity field.
type MinWindow struct {
	Start, End int
}

// Active reports whether minute-of-day m (0-1439) falls inside the window.
func (w MinWindow) Active(m int) bool {
	if w.Start <= w.End {
		return m >= w.Start && m < w.End
	}
	return m >= w.Start || m < w.End
}

// minOfDay returns t's minute-of-day (0-1439), for use with MinWindow.
func minOfDay(t time.Time) int { return t.Hour()*60 + t.Minute() }

// inAnyMinWindow reports whether m falls inside any of ws.
func inAnyMinWindow(ws []MinWindow, m int) bool {
	for _, w := range ws {
		if w.Active(m) {
			return true
		}
	}
	return false
}

func defaultStateDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "cs-sleeper")
	}
	return "/var/run/cs-sleeper"
}

func defaultLogFile() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "cs-sleeper", "cs-sleeper.log")
	}
	return "/var/log/cs-sleeper.log"
}

func defaultConfig() *Config {
	sd := defaultStateDir()
	return &Config{
		Enabled:    true,
		Wait:       600,
		Interval:   5,
		Policy:     "standby",
		StandbyMin: 10,
		Wake:       "on-access",
		Parallel:   4,
		VerifyIdle: 5,
		VMMode:     "off",
		PoolRescan: 60,
		StateDir:   sd,
		LogFile:    defaultLogFile(),
		LogLevel:   "info",
		PidFile:    filepath.Join(sd, "cs-sleeper.pid"),
	}
}

// defaultConfigPath returns the config file location (uniform /opt on all
// platforms, as used by the napp-it backend).
func defaultConfigPath() string {
	if p := os.Getenv("CS_SLEEPER_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(string(filepath.Separator), "opt", "csweb-gui", "_cfg", "cs-sleeper")
}

// loadConfig reads the config file at path (or the default location),
// creating it with defaults if it does not exist yet.
func loadConfig(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath()
	}
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("cannot create config dir: %w", err)
			}
			if err := os.WriteFile(path, marshalConfig(cfg), 0o644); err != nil {
				return nil, fmt.Errorf("cannot write default config: %w", err)
			}
			fmt.Fprintf(os.Stderr, "created default config at %s\n", path)
			return cfg, nil
		}
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	if err := parseConfig(string(data), cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseConfig(text string, c *Config) error {
	sc := bufio.NewScanner(strings.NewReader(text))
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("config line %d: expected key = value, got %q", lineNo, line)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		if i := strings.Index(val, "#"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		switch key {
		case "enabled":
			c.Enabled = parseBool(val, c.Enabled)
		case "hd", "disks":
			c.Disks = splitList(val)
		case "exclude":
			c.Exclude = splitList(val)
		case "pools":
			c.Pools = splitList(val)
		case "vm-mode":
			c.VMMode = val
		case "pool-rescan":
			c.PoolRescan = parseInt(val, c.PoolRescan)
		case "activity":
			ws, err := parseWindows(val)
			if err != nil {
				return fmt.Errorf("config line %d: %v", lineNo, err)
			}
			c.Activity = ws
		case "wait":
			c.Wait = parseInt(val, c.Wait)
		case "interval":
			c.Interval = parseInt(val, c.Interval)
		case "policy":
			c.Policy = val
		case "standby-min":
			c.StandbyMin = parseInt(val, c.StandbyMin)
		case "wake":
			c.Wake = val
		case "parallel":
			c.Parallel = parseInt(val, c.Parallel)
		case "verify-idle":
			c.VerifyIdle = parseInt(val, c.VerifyIdle)
		case "state-dir":
			c.StateDir = val
		case "log-file":
			c.LogFile = val
		case "log-level":
			c.LogLevel = val
		case "pid-file":
			c.PidFile = val
		case "export-pools":
			c.ExportPools = splitList(val)
		case "export-timetable":
			ws, err := parseHHMMWindows(val)
			if err != nil {
				return fmt.Errorf("config line %d: %v", lineNo, err)
			}
			c.ExportTimetable = ws
		case "pool-window":
			pw, err := parsePoolWindows(val)
			if err != nil {
				return fmt.Errorf("config line %d: %v", lineNo, err)
			}
			c.PoolWindow = pw
		case "active-timetable":
			ws, err := parseHHMMWindows(val)
			if err != nil {
				return fmt.Errorf("config line %d: %v", lineNo, err)
			}
			c.ActiveTimetable = ws
		default:
			return fmt.Errorf("config line %d: unknown key %q", lineNo, key)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return validateConfig(c)
}

// validateConfig cross-checks fields that parseConfig cannot validate
// line-by-line (a referenced pool might be declared on a later line than
// the field referencing it, and overlap checks need the whole window list
// at once). Called once after a full parse, from both the initial load and
// any config saved by a frontend such as csweb-gui -- so this is enforced
// no matter what wrote the file, not only when csweb-gui's own Settings
// form happens to validate on Save.
func validateConfig(c *Config) error {
	pools := make(map[string]bool, len(c.Pools))
	for _, p := range c.Pools {
		pools[p] = true
	}
	if err := validateNoOverlap(c.ExportTimetable); err != nil {
		return fmt.Errorf("export-timetable: %v", err)
	}
	if err := validateNoOverlap(c.ActiveTimetable); err != nil {
		return fmt.Errorf("active-timetable: %v", err)
	}
	for _, p := range c.ExportPools {
		if !pools[p] {
			return fmt.Errorf("export-pools: pool %q is not listed in pools", p)
		}
	}
	for p, ws := range c.PoolWindow {
		if !pools[p] {
			return fmt.Errorf("pool-window: pool %q is not listed in pools", p)
		}
		if err := validateNoOverlap(ws); err != nil {
			return fmt.Errorf("pool-window %q: %v", p, err)
		}
	}
	return nil
}

// validateNoOverlap rejects a window list containing two windows that
// overlap. A midnight-crossing window (End < Start) is split into its two
// non-crossing segments first, then every pair of segments is checked.
func validateNoOverlap(ws []MinWindow) error {
	type seg struct{ s, e int }
	var segs []seg
	for _, w := range ws {
		if w.Start <= w.End {
			segs = append(segs, seg{w.Start, w.End})
		} else {
			segs = append(segs, seg{w.Start, 1440}, seg{0, w.End})
		}
	}
	for i := 0; i < len(segs); i++ {
		for j := i + 1; j < len(segs); j++ {
			if segs[i].s < segs[j].e && segs[j].s < segs[i].e {
				return fmt.Errorf("overlapping windows")
			}
		}
	}
	return nil
}

// parseHHMM parses "HH:MM" (0-23 : 0-59) into minutes since midnight.
func parseHHMM(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return 0, fmt.Errorf("%q: expected HH:MM", s)
	}
	hh, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || hh < 0 || hh > 23 {
		return 0, fmt.Errorf("%q: bad hour", s)
	}
	mm, err := strconv.Atoi(strings.TrimSpace(m))
	if err != nil || mm < 0 || mm > 59 {
		return 0, fmt.Errorf("%q: bad minute", s)
	}
	return hh*60 + mm, nil
}

// parseHHMMWindows parses a comma list of "HH:MM-HH:MM" ranges.
func parseHHMMWindows(s string) ([]MinWindow, error) {
	var out []MinWindow
	for _, p := range splitList(s) {
		a, b, ok := strings.Cut(p, "-")
		if !ok {
			return nil, fmt.Errorf("%q: expected HH:MM-HH:MM", p)
		}
		start, err := parseHHMM(a)
		if err != nil {
			return nil, err
		}
		end, err := parseHHMM(b)
		if err != nil {
			return nil, err
		}
		out = append(out, MinWindow{Start: start, End: end})
	}
	return out, nil
}

// parsePoolWindows parses "pool:HH:MM-HH:MM,HH:MM-HH:MM;pool2:HH:MM-HH:MM"
// -- ";" separates per-pool entries, ":" separates a pool name from its
// window list, "," separates multiple windows for the same pool (same
// separator as everywhere else windows are comma-listed).
func parsePoolWindows(s string) (map[string][]MinWindow, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := map[string][]MinWindow{}
	for _, entry := range strings.Split(s, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		pool, windows, ok := strings.Cut(entry, ":")
		pool = strings.TrimSpace(pool)
		if !ok || pool == "" {
			return nil, fmt.Errorf("pool-window %q: expected pool:HH:MM-HH:MM", entry)
		}
		ws, err := parseHHMMWindows(windows)
		if err != nil {
			return nil, fmt.Errorf("pool-window %q: %v", pool, err)
		}
		out[pool] = ws
	}
	return out, nil
}

// formatHHMM formats minutes-since-midnight as "HH:MM".
func formatHHMM(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// formatMinWindows formats a window list as the "HH:MM-HH:MM,..." form
// parseHHMMWindows accepts.
func formatMinWindows(ws []MinWindow) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = formatHHMM(w.Start) + "-" + formatHHMM(w.End)
	}
	return strings.Join(parts, ",")
}

// formatPoolWindow formats a pool-window map as the "pool:...;pool2:..."
// form parsePoolWindows accepts, pools sorted for a stable, diffable file.
func formatPoolWindow(m map[string][]MinWindow) string {
	pools := make([]string, 0, len(m))
	for p := range m {
		pools = append(pools, p)
	}
	sort.Strings(pools)
	parts := make([]string, 0, len(pools))
	for _, p := range pools {
		parts = append(parts, p+":"+formatMinWindows(m[p]))
	}
	return strings.Join(parts, ";")
}

func marshalConfig(c *Config) []byte {
	var b strings.Builder
	b.WriteString("# cs-sleeper configuration -- see README.md for the full reference.\n")
	b.WriteString("# This file was created automatically with defaults.\n")
	fmt.Fprintf(&b, "enabled      = %s\n", boolStr(c.Enabled))
	fmt.Fprintf(&b, "disks        = %s\n", strings.Join(c.Disks, ","))
	fmt.Fprintf(&b, "exclude      = %s\n", strings.Join(c.Exclude, ","))
	fmt.Fprintf(&b, "pools        = %s\n", strings.Join(c.Pools, ","))
	fmt.Fprintf(&b, "activity     = %s\n", windowsStr(c.Activity))
	fmt.Fprintf(&b, "active-timetable = %s\n", formatMinWindows(c.ActiveTimetable))
	fmt.Fprintf(&b, "wait         = %d\n", c.Wait)
	fmt.Fprintf(&b, "interval     = %d\n", c.Interval)
	fmt.Fprintf(&b, "policy       = %s\n", c.Policy)
	fmt.Fprintf(&b, "standby-min  = %d\n", c.StandbyMin)
	fmt.Fprintf(&b, "wake         = %s\n", c.Wake)
	fmt.Fprintf(&b, "parallel     = %d\n", c.Parallel)
	fmt.Fprintf(&b, "verify-idle  = %d\n", c.VerifyIdle)
	fmt.Fprintf(&b, "vm-mode      = %s\n", c.VMMode)
	fmt.Fprintf(&b, "pool-rescan  = %d\n", c.PoolRescan)
	fmt.Fprintf(&b, "state-dir    = %s\n", c.StateDir)
	fmt.Fprintf(&b, "log-file     = %s\n", c.LogFile)
	fmt.Fprintf(&b, "log-level    = %s\n", c.LogLevel)
	fmt.Fprintf(&b, "pid-file     = %s\n", c.PidFile)
	fmt.Fprintf(&b, "export-pools     = %s\n", strings.Join(c.ExportPools, ","))
	fmt.Fprintf(&b, "export-timetable = %s\n", formatMinWindows(c.ExportTimetable))
	fmt.Fprintf(&b, "pool-window      = %s\n", formatPoolWindow(c.PoolWindow))
	return []byte(b.String())
}

func boolStr(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func windowsStr(ws []Window) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = fmt.Sprintf("%d-%d", w.Start, w.End)
	}
	return strings.Join(parts, ",")
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "true", "on", "1", "enable", "enabled":
		return true
	case "no", "false", "off", "0", "disable", "disabled":
		return false
	}
	return def
}

func parseInt(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}

func parseWindows(s string) ([]Window, error) {
	var out []Window
	for _, p := range splitList(s) {
		a, b, ok := strings.Cut(p, "-")
		if !ok {
			return nil, fmt.Errorf("activity %q: expected H-H", p)
		}
		start, err := strconv.Atoi(strings.TrimSpace(a))
		if err != nil || start < 0 || start > 23 {
			return nil, fmt.Errorf("activity %q: bad start hour", p)
		}
		end, err := strconv.Atoi(strings.TrimSpace(b))
		if err != nil || end < 0 || end > 23 {
			return nil, fmt.Errorf("activity %q: bad end hour", p)
		}
		out = append(out, Window{Start: start, End: end})
	}
	return out, nil
}

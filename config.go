package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
		default:
			return fmt.Errorf("config line %d: unknown key %q", lineNo, key)
		}
	}
	return sc.Err()
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

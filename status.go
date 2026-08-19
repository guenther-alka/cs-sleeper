package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/replcheck"
	"github.com/guenther-alka/cs-sleeper/internal/sysio"
	"github.com/guenther-alka/cs-sleeper/internal/zfs"
)

// statusCmd prints a one-shot report of managed disks and their state.
func statusCmd(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "", "config file (default /opt/csweb-gui/_cfg/cs-sleeper)")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if *asJSON {
		printStatusJSON(cfg)
		return
	}

	devices := managedDevices(cfg)
	rep := replcheck.Check()

	fmt.Println("cs-sleeper " + version)
	fmt.Println()
	fmt.Printf("config: enabled=%v wait=%ds interval=%ds policy=%s wake=%s standby-min=%dm verify-idle=%d\n",
		cfg.Enabled, cfg.Wait, cfg.Interval, cfg.Policy, cfg.Wake, cfg.StandbyMin, cfg.VerifyIdle)
	fmt.Printf("managed disks: %s\n", strings.Join(devices, ","))
	fmt.Printf("exclude:       %s\n", strings.Join(cfg.Exclude, ","))
	fmt.Printf("replication in flight: %v\n", rep.Active)
	fmt.Println()
	fmt.Printf("%-16s %-12s %s\n", "pool", "state", "disks")
	for _, p := range cfg.Pools {
		imported, _ := zfs.IsImported(p)
		state := "not-imported"
		if imported {
			state = "imported"
		}
		data := zfs.DisksOfPoolSafe(p)
		flash := zfs.NeverSleepDisks(p)
		line := strings.Join(data, ",")
		if len(flash) > 0 {
			line += "  (flash: " + strings.Join(flash, ",") + ")"
		}
		fmt.Printf("%-16s %-12s %s\n", p, state, line)
	}
	fmt.Println()

	st := readStateFile(cfg)
	if st != nil {
		fmt.Printf("daemon: pid=%d updated=%s\n", st.PID, st.Updated.Format("15:04:05"))
		fmt.Println()
		fmt.Printf("%-16s %-9s %-12s %s\n", "device", "sleeping", "idle", "last-active")
		for _, d := range st.Disks {
			fmt.Printf("%-16s %-9v %-12s %s\n", d.Device, d.Sleeping, durStr(d.IdleSeconds), fmtTime(d.LastActive))
		}
	} else {
		fmt.Println("daemon: not running (no state file found)")
	}
	fmt.Println()

	reader := sysio.NewReader()
	first := sample(reader, 1)
	time.Sleep(time.Second)
	second := sample(reader, 1)

	fmt.Println("live I/O sample:")
	fmt.Printf("%-16s %-8s %-12s %-12s\n", "device", "active", "read-ops", "write-ops")
	for _, d := range devices {
		fmt.Printf("%-16s %-8v %-12d %-12d\n", d, sysio.Active(first[d], second[d]), second[d].ReadIO, second[d].WriteIO)
	}

	if tasks, err := loadSchedule(cfg); err == nil && len(tasks) > 0 {
		fmt.Println("scheduled tasks:")
		for _, t := range tasks {
			fmt.Printf("  %-9s %-16s at %s\n", t.Kind, t.Pool, t.At.Format("2006-01-02 15:04"))
		}
	}
}

func readStateFile(cfg *Config) *stateJSON {
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, "state.json"))
	if err != nil {
		return nil
	}
	var st stateJSON
	if err := json.Unmarshal(b, &st); err != nil {
		return nil
	}
	return &st
}

func printStatusJSON(cfg *Config) {
	out := map[string]any{
		"version":     version,
		"enabled":     cfg.Enabled,
		"config":      cfg,
		"repl_active": replcheck.Check().Active,
	}
	if pools, err := zfs.Pools(); err == nil {
		out["pools"] = pools
	}
	if st := readStateFile(cfg); st != nil {
		out["state"] = st
	}
	if tasks, err := loadSchedule(cfg); err == nil && len(tasks) > 0 {
		out["schedule"] = tasks
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}

func durStr(sec int) string {
	if sec <= 0 {
		return "-"
	}
	d := time.Duration(sec) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", sec)
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

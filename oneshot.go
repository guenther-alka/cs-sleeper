package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/replcheck"
	"github.com/guenther-alka/cs-sleeper/internal/sleeper"
	"github.com/guenther-alka/cs-sleeper/internal/zfs"
)

// diskCmd implements `sleepnow` / `wakeupnow`.
func diskCmd(args []string, action string) {
	fs := flag.NewFlagSet(action+"now", flag.ExitOnError)
	disk := fs.String("disk", "", "device name (required)")
	configPath := fs.String("config", "", "config file (default /opt/csweb-gui/_cfg/cs-sleeper)")
	fs.Parse(args)

	if *disk == "" {
		fmt.Fprintln(os.Stderr, "error: --disk is required")
		os.Exit(2)
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if action == "sleep" {
		if err := guardDevice(cfg, *disk); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if rep := replcheck.Check(); rep.Active {
			fmt.Fprintf(os.Stderr, "refusing to sleep %s: zfs send/receive in flight\n", *disk)
			os.Exit(1)
		}
	}

	var out string
	if action == "sleep" {
		out, err = sleeper.Sleep(*disk)
	} else {
		out, err = sleeper.Wake(*disk)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s failed: %v\n%s\n", action, *disk, err, strings.TrimSpace(out))
		os.Exit(1)
	}
	fmt.Printf("%s %s: ok\n%s\n", action, *disk, strings.TrimSpace(out))
}

// poolCmd implements `import-now` / `export-now`.
func poolCmd(args []string, action string) {
	fs := flag.NewFlagSet(action+"-now", flag.ExitOnError)
	pool := fs.String("pool", "", "pool name (required)")
	force := fs.Bool("force", false, "force (passes -f to zpool)")
	configPath := fs.String("config", "", "config file")
	fs.Parse(args)

	if *pool == "" {
		fmt.Fprintln(os.Stderr, "error: --pool is required")
		os.Exit(2)
	}
	if _, err := loadConfig(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if action == "export" {
		if err := guardBootPool(*pool); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if rep := replcheck.Check(); rep.Active {
			fmt.Fprintf(os.Stderr, "refusing to export %s: zfs send/receive in flight\n", *pool)
			os.Exit(1)
		}
	}

	var out string
	var err error
	if action == "export" {
		out, err = zfs.Export(*pool, *force)
	} else {
		out, err = zfs.Import(*pool, *force)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s failed: %v\n%s\n", action, *pool, err, strings.TrimSpace(out))
		os.Exit(1)
	}
	fmt.Printf("%s %s: ok\n%s\n", action, *pool, strings.TrimSpace(out))
}

// sleepPoolCmd implements `sleeppool`.
func sleepPoolCmd(args []string) {
	fs := flag.NewFlagSet("sleeppool", flag.ExitOnError)
	pool := fs.String("pool", "", "pool name (required)")
	at := fs.String("at", "now", "when to sleep: now or HH:MM (queued for the daemon)")
	export := fs.Bool("export", false, "export the pool (backup pools) instead of standby")
	includeVM := fs.Bool("include-vm", false, "suspend/shutdown VMs on the pool first (vm-mode)")
	force := fs.Bool("force", false, "force (passes -f to zpool export/import)")
	configPath := fs.String("config", "", "config file")
	fs.Parse(args)

	if *pool == "" {
		fmt.Fprintln(os.Stderr, "error: --pool is required")
		os.Exit(2)
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	when, err := parseAt(*at)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if !when.IsZero() {
		if err := queueTask(cfg, scheduledTask{Kind: taskSleepPool, Pool: *pool, At: when, Export: *export, IncludeVM: *includeVM, Force: *force}); err != nil {
			fmt.Fprintln(os.Stderr, "error: cannot queue task:", err)
			os.Exit(1)
		}
		fmt.Printf("sleeppool %s queued for %s\n", *pool, when.Format("15:04"))
		return
	}

	// FOUND LIVE cs_26.08.20 (Gea report: "bei Pool Sleep per Menü kommt nur
	// reload"): this logger used to write to os.Stderr, so csweb-gui's
	// remote exec of this one-shot command -- which reads the process's
	// stdout -- never saw the per-disk detail lines (e.g. "sleeppool
	// daten1: c6t...: exit status 1"), only the unconditional final
	// "sleeppool daten1: ok" below. Combined with execSleepPool previously
	// never returning an error for partial disk failures, the GUI's own
	// failure-detection regex (_sleeper_show_result_or_reload) had nothing
	// to match and silently reloaded. Logging to stdout instead -- same
	// stream status/enable/disable already use successfully -- means the
	// detail lines and the now-possible non-zero exit both reach the GUI.
	logger := log.New(os.Stdout, "", log.LstdFlags)
	if err := execSleepPool(cfg, *pool, *export, *includeVM, *force, logger); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	fmt.Printf("sleeppool %s: ok\n", *pool)
}

// wakePoolCmd implements `wakepool`.
func wakePoolCmd(args []string) {
	fs := flag.NewFlagSet("wakepool", flag.ExitOnError)
	pool := fs.String("pool", "", "pool name (required)")
	at := fs.String("at", "now", "when to wake: now or HH:MM (queued for the daemon)")
	includeVM := fs.Bool("include-vm", false, "resume/start VMs on the pool (vm-mode)")
	force := fs.Bool("force", false, "force (passes -f to zpool import)")
	configPath := fs.String("config", "", "config file")
	fs.Parse(args)

	if *pool == "" {
		fmt.Fprintln(os.Stderr, "error: --pool is required")
		os.Exit(2)
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	when, err := parseAt(*at)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if !when.IsZero() {
		if err := queueTask(cfg, scheduledTask{Kind: taskWakePool, Pool: *pool, At: when, IncludeVM: *includeVM, Force: *force}); err != nil {
			fmt.Fprintln(os.Stderr, "error: cannot queue task:", err)
			os.Exit(1)
		}
		fmt.Printf("wakepool %s queued for %s\n", *pool, when.Format("15:04"))
		return
	}

	// Same stdout fix as sleepPoolCmd above -- see that comment.
	logger := log.New(os.Stdout, "", log.LstdFlags)
	if err := execWakePool(cfg, *pool, *includeVM, *force, logger); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	fmt.Printf("wakepool %s: ok\n", *pool)
}

// parseAt parses "now" (zero time = immediate) or "HH:MM" (local time today,
// or tomorrow if already passed).
func parseAt(s string) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "now" {
		return time.Time{}, nil
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --at %q (want now or HH:MM)", s)
	}
	now := time.Now()
	at := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
	if !at.After(now) {
		at = at.Add(24 * time.Hour)
	}
	return at, nil
}

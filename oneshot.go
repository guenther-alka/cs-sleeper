package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

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
	if _, err := loadConfig(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if action == "sleep" {
		if rep := replcheck.Check(); rep.Active {
			fmt.Fprintf(os.Stderr, "refusing to sleep %s: zfs send/receive in flight\n", *disk)
			os.Exit(1)
		}
	}

	var out string
	var err error
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

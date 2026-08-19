// cs-sleeper -- spin down idle ZFS pool disks and wake them on demand.
//
// A small, dependency-free daemon that watches per-disk I/O activity and
// puts disks that have been idle past a configurable `wait` into standby
// (via smartctl). It refuses to touch disks while a `zfs send`/`receive`
// is in flight, and provides guarded one-shot commands to spin disks
// down/up and to export/import whole pools.
//
// See README.md for the full concept, config reference and platform matrix.
package main

import (
	"fmt"
	"os"
)

// version is the release version; set at build time via
// -ldflags "-X main.version=..." when tagging a release.
var version = "1.1.0-rc1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("cs-sleeper " + version)
	case "daemon":
		daemonCmd(os.Args[2:])
	case "status":
		statusCmd(os.Args[2:])
	case "enable":
		enableCmd(os.Args[2:])
	case "disable":
		disableCmd(os.Args[2:])
	case "sleepnow":
		diskCmd(os.Args[2:], "sleep")
	case "wakeupnow":
		diskCmd(os.Args[2:], "wake")
	case "import-now":
		poolCmd(os.Args[2:], "import")
	case "export-now":
		poolCmd(os.Args[2:], "export")
	case "sleeppool":
		sleepPoolCmd(os.Args[2:])
	case "wakepool":
		wakePoolCmd(os.Args[2:])
	case "help", "-h", "--help", "-help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`cs-sleeper -- spin down idle ZFS pool disks (v1.1.0-rc1)

Usage:
  cs-sleeper daemon [--config PATH] [--foreground] [--once]
  cs-sleeper status [--config PATH] [--json]
  cs-sleeper enable   [--config PATH]
  cs-sleeper disable  [--config PATH]
  cs-sleeper sleepnow --disk NAME [--config PATH]
  cs-sleeper wakeupnow --disk NAME [--config PATH]
  cs-sleeper sleeppool --pool NAME [--export] [--include-vm] [--at now|HH:MM] [--force]
  cs-sleeper wakepool  --pool NAME [--include-vm] [--at now|HH:MM] [--force]
  cs-sleeper import-now --pool NAME [--config PATH] [--force]
  cs-sleeper export-now --pool NAME [--config PATH] [--force]
  cs-sleeper version

Commands:
  daemon      run the idle-detection loop (background unless --foreground)
  status      one-shot report of managed pools, disks and their state
  enable      persist enabled=yes and start the daemon
  disable     persist enabled=no and stop the daemon
  sleepnow    spin one disk down immediately (refused while zfs send/receive runs)
  wakeupnow   spin one disk up immediately
  sleeppool   put a whole pool to sleep (standby by default, --export for backup pools)
  wakepool    wake a pool (import if needed) and optionally its VMs
  import-now  import one ZFS pool (guarded)
  export-now  export one ZFS pool (refused while busy / replication runs)

Config:
  default /opt/csweb-gui/_cfg/cs-sleeper (created with defaults if missing);
  override with --config PATH or env CS_SLEEPER_CONFIG.

  enabled    = yes        # daemon runs at all
  pools      = pool1,pool2 # pools whose disks are managed (via zpool status)
  disks      = sda,ada0   # additional free disks to manage directly
  exclude    =            # disks to never touch
  activity   = 12-14,18-6 # I/O windows; sleep is allowed OUTSIDE them
  wait       = 600        # seconds idle before a disk sleeps
  interval   = 5          # sampling interval (seconds)
  policy     = standby    # sleep policy (standby)
  standby-min= 10         # drive-internal standby timer set at start (min)
  wake       = on-access  # on-access | manual
  parallel   = 4          # max concurrent sleep/wake operations
  verify-idle= 5          # consecutive idle samples before sleeping
  vm-mode    = off        # off | proxmox_suspend | proxmox_shutdown
  pool-rescan= 60         # seconds between pool disk re-resolution
  state-dir  = /var/run/cs-sleeper
  log-file   = /var/log/cs-sleeper.log
  log-level  = info       # debug | info | warn | error
  pid-file   = /var/run/cs-sleeper/cs-sleeper.pid

Never slept: the OS boot disk, SLOG/L2ARC/special/dedup flash disks, and
everything listed in exclude.

Requires smartctl in PATH (and zpool/zfs on the managed host).`)
}

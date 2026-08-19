# cs-sleeper

Spin down idle ZFS pool disks and wake them on demand.

`cs-sleeper` is a small, dependency-free daemon (single static Go binary) that
watches per-disk I/O activity and puts disks that have been idle for a
configurable `wait` period into standby via `smartctl`. It refuses to touch a
disk while a `zfs send`/`receive` is in flight, so a running replication job is
never interrupted. It also ships guarded one-shot commands to spin a single
disk down/up and to export/import a whole pool.

`cs-sleeper` is part of the napp-it / csweb-gui tool family and is designed to
run on ZFS hosts: Linux, illumos, Solaris, FreeBSD (and Windows as a
smartctl-only target).

---

## Table of contents

- [Concept](#concept)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [Command line reference](#command-line-reference)
- [Platform matrix](#platform-matrix)
- [Safety model](#safety-model)
- [napp-it / csweb-gui integration](#napp-it--csweb-gui-integration)
- [Exit codes](#exit-codes)
- [License](#license)

---

## Concept

The daemon samples per-disk I/O once per `interval` (default 5 s):

1. It detects activity by comparing successive counters (Linux) or by reading
   the interval rate (illumos/Solaris/FreeBSD/Windows).
2. A disk that shows no I/O accumulates idle time; once it has been idle for
   `wait` seconds and has passed `verify-idle` consecutive idle re-checks, the
   daemon issues `smartctl -s standby,now` to spin it down.
3. Sleep is only allowed **outside** the configured `activity` windows (for
   example "do not sleep between 12:00-14:00 and 18:00-06:00" when backups or
   replication are expected to run).
4. Before sleeping, and before any `export-now`, the daemon checks for a
   running `zfs send`/`receive`; if one is found, the action is skipped.

As a belt-and-suspenders fallback the daemon also sets the drive-internal
standby timer (`smartctl -s standby,<standby-min>`) at startup, so disks still
spin down on their own even if the daemon is stopped.

`cs-sleeper` only manages the disks you list in `hd`. It never touches disks in
`exclude`, system disks, or disks that are not listed.

---

## Requirements

- A supported OS (see [Platform matrix](#platform-matrix)).
- `smartctl` (smartmontools) in `PATH`, with root/administrator privileges.
- `zpool` and `zfs` in `PATH` on the managed host (for the replication safety
  gate and the import/export commands).
- No other runtime dependencies; the binary is fully static
  (`CGO_ENABLED=0`).

---

## Installation

Prebuilt binaries are attached to every GitHub release. The layout mirrors the
other napp-it tools so the csweb-gui web interface can download them:

```
cs-sleeper-mswin.amd64.tar.gz      -> mswin.amd64/cs-sleeper.exe
cs-sleeper-linux.amd64.tar.gz      -> linux.amd64/cs-sleeper
cs-sleeper-linux.arm64.tar.gz      -> linux.arm64/cs-sleeper
cs-sleeper-illumos.amd64.tar.gz    -> illumos.amd64/cs-sleeper
cs-sleeper-solaris.amd64.tar.gz    -> solaris.amd64/cs-sleeper
cs-sleeper-freebsd.amd64.tar.gz    -> freebsd.amd64/cs-sleeper
cs-sleeper-darwin.amd64.tar.gz     -> darwin.amd64/cs-sleeper
cs-sleeper-darwin.arm64.tar.gz     -> darwin.arm64/cs-sleeper
```

For a manual install, copy the binary for your platform to a location in
`PATH` (for example `/usr/local/bin/cs-sleeper`) and make it executable:

```sh
chmod +x /usr/local/bin/cs-sleeper
```

The napp-it backend expects the tool under
`.../data/cs_server/tools/cs-sleeper/<os>.<arch>/cs-sleeper[.exe]`; see
[napp-it integration](#napp-it--csweb-gui-integration).

---

## Quick start

```sh
# 1. Show the created config path and current defaults (creates the file).
cs-sleeper status

# 2. Edit the config and list the disks to manage.
vi /opt/csweb-gui/_cfg/cs-sleeper
#   hd = sda,ada0,c2t1d0      (device names, see Platform matrix)

# 3. Run once in the foreground to verify detection.
cs-sleeper daemon --foreground --once

# 4. Run as a daemon (background, pid-file + state file).
cs-sleeper daemon

# 5. Inspect state.
cs-sleeper status

# 6. Manual control.
cs-sleeper sleepnow  --disk sda
cs-sleeper wakeupnow --disk sda
cs-sleeper export-now --pool tank
cs-sleeper import-now --pool tank
```

---

## Configuration

Config file: `/opt/csweb-gui/_cfg/cs-sleeper` (the same `_cfg` directory used
by the other napp-it tools). It is created automatically with defaults the
first time any command runs. Override the location with `--config PATH` or the
environment variable `CS_SLEEPER_CONFIG`.

Format: one `key = value` per line; `#` or `;` start a comment; empty lines are
ignored. Boolean values accept `yes/no`, `true/false`, `on/off`, `1/0`.

| Key          | Default                 | Meaning                                                                 |
|--------------|-------------------------|-------------------------------------------------------------------------|
| `enabled`    | `yes`                   | Whether the daemon runs at all. `no` makes `daemon` exit immediately.   |
| `hd`         | *(empty)*               | Comma-separated disks to manage (device names, see Platform matrix).     |
| `exclude`    | *(empty)*               | Comma-separated disks to never touch.                                    |
| `pools`      | *(empty)*               | Pools considered for import/export safety (informational).               |
| `activity`   | *(empty)*               | Comma-separated hour ranges `H-H` (0-23, may cross midnight). Sleep is   |
|              |                         | allowed **only outside** these windows. Empty = sleep anytime.           |
| `wait`       | `600`                   | Seconds a disk must be idle before it is put to sleep.                   |
| `interval`   | `5`                     | Sampling interval in seconds.                                            |
| `policy`     | `standby`               | Sleep policy. Only `standby` (smartctl spin-down) is implemented in v1.0.|
| `standby-min`| `10`                    | Drive-internal standby timer (minutes) set at daemon start as fallback.  |
| `wake`       | `on-access`             | `on-access` (track wake-ups) or `manual` (use `wakeupnow`).              |
| `parallel`   | `4`                     | Max concurrent sleep/wake operations.                                    |
| `verify-idle`| `5`                     | Consecutive idle samples required (after `wait`) before sleeping.        |
| `state-dir`  | `/var/run/cs-sleeper`   | Directory for the pid file and `state.json`.                             |
| `log-file`   | `/var/log/cs-sleeper.log` | Daemon log file.                                                       |
| `log-level`  | `info`                  | `debug` / `info` / `warn` / `error`.                                     |
| `pid-file`   | `/var/run/cs-sleeper/cs-sleeper.pid` | Pid file (prevents duplicate daemons).                       |

Example:

```
enabled    = yes
hd         = sda,sdb,ada0,ada1
exclude    =
pools      = tank,backup
activity   = 12-14,18-6
wait       = 600
interval   = 5
policy     = standby
standby-min= 10
wake       = on-access
parallel   = 4
verify-idle= 5
```

---

## Command line reference

```
cs-sleeper daemon [--config PATH] [--foreground] [--once]
cs-sleeper status [--config PATH] [--json]
cs-sleeper sleepnow  --disk NAME [--config PATH]
cs-sleeper wakeupnow --disk NAME [--config PATH]
cs-sleeper import-now --pool NAME [--config PATH] [--force]
cs-sleeper export-now --pool NAME [--config PATH] [--force]
cs-sleeper version
```

| Command     | Flags                    | Description                                                          |
|-------------|--------------------------|----------------------------------------------------------------------|
| `daemon`    | `--config`, `--foreground`, `--once` | Idle-detection loop. Background by default; `--foreground` logs to stdout; `--once` runs one sample and exits (diagnostics). |
| `status`    | `--config`, `--json`     | One-shot report: config, managed disks, daemon state, live I/O sample. `--json` emits machine-readable JSON. |
| `sleepnow`  | `--disk`, `--config`     | Spin one disk down immediately; refused while `zfs send/receive` runs. |
| `wakeupnow` | `--disk`, `--config`     | Spin one disk up immediately.                                         |
| `import-now`| `--pool`, `--config`, `--force` | Import one pool (`zpool import`).                                |
| `export-now`| `--pool`, `--config`, `--force` | Export one pool (`zpool export`); refused while replication runs. |
| `version`   |                          | Print the version string.                                             |

The daemon writes its per-disk state to `<state-dir>/state.json` after every
sample; `status` reads it and also performs a fresh 1-second I/O sample for a
live view.

---

## Platform matrix

| OS        | Device names           | I/O source                    | Sleep / wake                    |
|-----------|------------------------|-------------------------------|---------------------------------|
| Linux     | `sda`, `nvme0n1`       | `/proc/diskstats` (cumulative)| `smartctl -s standby,now /dev/<name>` |
| illumos   | `c2t1d0`               | `iostat -xn <n> 2` (rate)     | `smartctl ... /dev/rdsk/<name>s2` |
| Solaris   | `c2t1d0`               | `iostat -xn <n> 2` (rate)     | `smartctl ... /dev/rdsk/<name>s2` |
| FreeBSD   | `ada0`, `da0`          | `iostat -x -w <n> -c 2` (rate)| `smartctl -s standby,now /dev/<name>` |
| Windows   | `PhysicalDisk0`        | PowerShell `Get-Counter` (rate)| `smartctl ... PhysicalDriveN` / OpenZFS `/dev/sdN` |
| macOS     | `disk0`                | none (compile-only)            | `smartctl` via one-shot commands only |

Notes:

- Use the device names exactly as the OS shows them (`lsblk`, `zpool status`,
  `iostat -n`, `camcontrol devlist`, `Get-PhysicalDisk`). `/dev/` prefixes are
  optional and are stripped automatically; partition/slice suffixes
  (`sda1`, `ada0s1`, `c2t1d0s2`) are normalized to the whole disk.
- On illumos/Solaris configure the logical names shown by `iostat -n`
  (for example `c2t1d0`), which are the same names `zpool status` uses.
- On Windows idle detection uses the `PhysicalDisk N` performance counters;
  smartctl needs the corresponding `PhysicalDriveN` or OpenZFS `/dev/sdN` alias
  for the actual spin-down.
- macOS is a build target only; there is no idle detection, but
  `sleepnow`/`wakeupnow` work if a device is passed explicitly.

---

## Safety model

`cs-sleeper` is intentionally conservative:

1. **Replication gate** — before sleeping a disk or exporting a pool it checks
   for a running `zfs send`/`zfs receive` (via `pgrep -f`). If one is found the
   action is skipped for this tick and logged.
2. **Idle detection** — a disk is only slept after `wait` seconds without any
   I/O plus `verify-idle` consecutive idle samples, avoiding false triggers
   from transient activity.
3. **Activity windows** — sleep is refused inside the configured `activity`
   windows, so maintenance/replication slots are always respected.
4. **Explicit allow-list** — only disks listed in `hd` (and not in `exclude`)
   are ever managed.
5. **No forced pool actions by default** — `export-now`/`import-now` only pass
   `-f` to `zpool` when `--force` is given, and refuse while replication runs.

Caveat: `smartctl` spin-down on some controllers needs extra `-d` options
(for example USB or specific HBAs). If `smartctl` fails, the error is logged
and the disk is left alone; configure the drive so smartctl sees it, or use the
host's native tool.

---

## napp-it / csweb-gui integration

- Config: `/opt/csweb-gui/_cfg/cs-sleeper` (created on first use).
- Binary: `.../data/cs_server/tools/cs-sleeper/<os>.<arch>/cs-sleeper[.exe]`.
- Autostart: the csweb-gui backend starts the daemon from
  `server_boot_tasks.pl` (idempotent via the pid file and the `enabled` flag).
- Menu: `System > Services > Sleeper` exposes `status`, `sleepnow`,
  `wakeupnow`, `import-now` and `export-now`.

The one-shot commands (`sleepnow`, `wakeupnow`, `import-now`, `export-now`) do
not need autostart; only `daemon` does.

---

## Exit codes

| Code | Meaning                            |
|------|------------------------------------|
| 0    | Success (or daemon disabled)       |
| 1    | Error (config, smartctl, zpool, refused action) |
| 2    | Usage error (missing required flag)|

---

## License

BSD 2-Clause. See [LICENSE](LICENSE).



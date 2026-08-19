# Changelog

All notable changes to cs-sleeper are documented here. Versions follow
`v<major>.<minor>.<patch>`; see the git tags for the full history.

## v1.1.0-rc1 (2026-08-19) — Release Candidate

- Pool-level sleep/wake: new `sleeppool`/`wakepool` commands (standby, or
  `--export` for backup pools), with optional VM pause/resume/shutdown via the
  Proxmox backend (`vm-mode`).
- `sleeppool` flushes pending writes before spinning disks down (`zpool sync`,
  POSIX `sync` fallback on illumos/Solaris) and re-verifies that no new I/O
  arrived during the flush, so disks stay asleep instead of waking on the next
  transaction-group commit.
- Scheduled actions: `sleeppool`/`wakepool` accept `--at now|HH:MM`; the daemon
  runs due tasks from its schedule (timer windows).
- Enable/disable: `enable`/`disable` persist the `enabled` flag and start/stop
  the daemon.
- macOS: real idle detection via `iostat -d` (was compile-only), plus boot-disk
  detection (resolves the APFS physical store to the backing disk) and
  `smartctl` spin-down for mechanical disks; smartctl is auto-located in
  `/usr/local/sbin` when it is not on `PATH`.
- Safety: never sleep the OS boot disk, the boot pool's disks, or SLOG/L2ARC/
  special/dedup flash devices; hot spares remain managed.
- `status` reports a per-pool view (import state + data vs flash disks).
- Config keys added: `vm-mode`, `pool-rescan`; `pools` now manages member disks
  via `zpool status`.

## v1.0.0 (2026-08-19)

- Initial release: disk-idle sleeper for ZFS hosts.
- Commands: `daemon`, `status`, `sleepnow`, `wakeupnow`, `import-now`,
  `export-now`, `version`.
- Config at `/opt/csweb-gui/_cfg/cs-sleeper` (created with defaults if missing).
- Per-OS I/O readers: Linux (`/proc/diskstats`), illumos/Solaris
  (`iostat -xn`), FreeBSD (`iostat -x`), Windows (PowerShell `Get-Counter`),
  macOS (compile-only).
- ZFS safety: refuses to sleep disks or export pools while a
  `zfs send`/`receive` is in flight.
- Activity windows, idle `wait`, verify-idle re-checks, drive-internal
  standby-timer fallback, parallel sleep operations.
- 8-platform release build (mswin/linux/illumos/solaris/freebsd/darwin,
  amd64 + linux/darwin arm64).

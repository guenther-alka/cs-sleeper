# Changelog

All notable changes to cs-sleeper are documented here. Versions follow
`v<major>.<minor>.<patch>`; see the git tags for the full history.

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

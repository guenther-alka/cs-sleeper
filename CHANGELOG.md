# Changelog

All notable changes to cs-sleeper are documented here. Versions follow
`v<major>.<minor>.<patch>`; see the git tags for the full history.

## v1.1.0-rc3 (2026-08-19) — Release Candidate

Security/robustness follow-up to the rc2 review, implementing all five
recommendations:

- **Safety-net gap closed:** `sleepnow`, `sleeppool` and `export-now` now all
  refuse to touch the never-sleep set (OS boot disk, boot pool disks, SLOG/
  L2ARC/special/dedup flash devices, `exclude`) -- previously this was only
  enforced by the continuous daemon loop. `sleeppool`/`export-now` also
  refuse outright when the given `--pool` is the boot pool.
- **External tools resolved via fixed paths first:** `smartctl`, `zpool`,
  `qm`, `pgrep`, `iostat`, `findmnt`, `lsblk`, `df`, `diskutil`, `sync`,
  `powershell`, `taskkill` and `tasklist` are now looked up in well-known
  absolute install locations before falling back to a `$PATH` search
  (new `internal/xpath` package), closing a PATH-hijack risk given
  cs-sleeper normally runs as root/Administrator.
- **Disk/pool name validation:** `sysio.Valid` rejects names starting with
  `-` before they reach `smartctl`/`zpool` as a bare CLI argument (the
  Windows `DevicePath` path had no such guard before).
- **Timeouts everywhere:** every external command invocation (`zpool`, `qm`,
  `iostat`/`Get-Counter`, `pgrep`, `findmnt`/`lsblk`, `df`/`diskutil`,
  `sync`, `taskkill`/`tasklist`) now runs under a `context`-based timeout,
  not just `smartctl` as before -- an unresponsive device or tool can no
  longer wedge the daemon loop indefinitely.
- **Release integrity:** the release workflow now publishes a
  `checksums.txt` (SHA-256) alongside the `.tar.gz` archives, and all
  GitHub Actions steps are pinned to a commit SHA (with the version as a
  comment) instead of a floating major-version tag.

## v1.1.0-rc2 (2026-08-19) — Release Candidate

- `wake = on-access` now logs/tracks disks that wake on access (shown as
  `last-wake` in `status`); `manual` disables the tracking.
- `parallel = 0` now means unlimited concurrent sleep/wake operations (was:
  silently forced to sequential).

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

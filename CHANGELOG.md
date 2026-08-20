# Changelog

All notable changes to cs-sleeper are documented here. Versions follow
`v<major>.<minor>.<patch>`; see the git tags for the full history.

## v1.1.0-rc9 (2026-08-20) — Release Candidate

Bug fixes (live-caught via Gea's csweb-gui sleep test on member .203, pool
`daten1`):

- **illumos/Solaris: smartctl "Unable to detect device type" on every
  managed disk.** `sysio.Normalize()` lower-cases every device name for
  cross-platform comparison, but illumos' `/dev/rdsk/` symlinks embed the
  disk's WWN in mixed/upper-case hex (e.g. `c6t5000CCA0BBE3CE1Cd0`) --
  building the smartctl path directly from the lower-cased name
  (`c6t5000cca0bbe3ce1cd0`) produced a filename that doesn't exist on
  illumos' case-sensitive `/dev` tree, so `sleepnow`/`sleeppool`/the daemon
  loop could never actually put a disk to standby, only report a confusing
  smartctl autodetect failure. Confirmed live: `iostat -xn` itself already
  reports the correct upper-case name, so `Normalize()`'s lower-casing was
  the only source of the mismatch. Fix: `DevicePath()` (illumos/Solaris
  only) now resolves the real on-disk casing via a case-insensitive scan of
  `/dev/rdsk` before building the smartctl path; `Normalize()` itself is
  unchanged (still the right comparison key for exclude-lists/maps
  elsewhere).
- **`sleeppool`/`wakepool` reported "ok" (exit 0) even when individual
  disks failed to sleep/wake.** `execSleepPool`/`execWakePool` logged each
  disk's smartctl failure but always returned `nil`, so a partially-failed
  pool action looked like unqualified success to every caller. Both now
  return an aggregate error when any disk failed. Also moved the one-shot
  `sleeppool`/`wakepool` CLI commands' detail logger from stderr to stdout
  (matching `status`/`enable`/`disable`), so csweb-gui's Sleeper menu
  reliably sees the per-disk failure text instead of just the final summary
  line.

## v1.1.0-rc7 (2026-08-20) — Release Candidate

New feature (config-only, backward compatible):

- **`active-timetable`**, an HH:MM-granularity successor to the legacy
  hour-only `activity` field. Same role -- the global allow-sleep window
  for every disk that has no `pool-window` override -- just finer
  grained, matching `export-timetable`'s own HH:MM syntax (same
  `parseHHMMWindows`/`formatMinWindows` helpers, same overlap
  validation). When set, `active-timetable` takes over completely from
  `activity` (not merged with it) via the new `Config.SleepAllowed(t
  time.Time) bool` method, which `daemon.go`'s `AllowSleep` closure now
  calls instead of `InWindow(t.Hour())` directly. `activity` itself is
  unchanged and still fully functional -- kept as the fallback for
  hand-edited configs that still use the old hour-only syntax; it is
  simply no longer the field csweb-gui's own Settings form writes to.
- New tests: `SleepAllowed` prefers `active-timetable` over `activity`
  when both are set; `active-timetable` overlap validation;
  `active-timetable` config-key parsing.

## v1.1.0-rc6 (2026-08-20) — Release Candidate

New feature (config-only, backward compatible -- existing configs behave
identically since the new fields default empty/inert):

- **Per-pool allow-sleep window (`pool-window`).** Previously the single
  global `activity` window applied uniformly to every managed disk. A pool
  can now get its own allow-sleep window via `pool-window =
  pool:HH:MM-HH:MM,...;pool2:HH:MM-HH:MM`, which *replaces* (does not add to)
  `activity` for that pool's member disks only; a pool with no entry, and any
  free/standalone disk listed under `disks` (which has no owning pool),
  keeps using the original global `activity` window unchanged. Internally,
  `sleeper.Options.AllowSleep` changed signature from `func(time.Time) bool`
  to `func(device string, t time.Time) bool` so the idle-detection engine can
  look up the right window per disk; the new `devicePoolMap()` helper
  resolves each managed disk to its owning pool (rebuilt on every
  `pool-rescan` tick alongside the existing device-list rescan).
- **Forced export/import schedule for backup pools (`export-pools` +
  `export-timetable`).** A shared HH:MM timetable (`export-timetable =
  07:00-19:00,...`) applied to every pool listed in `export-pools`: outside
  every window the pool is force-exported (equivalent to `sleeppool
  --export`), inside a window it is imported and woken (equivalent to
  `wakepool`) -- independent of and in addition to ordinary idle-based
  sleep. Both fields must be non-empty for the feature to do anything for a
  given pool (a pool listed with no timetable is inert, not "always
  exported" -- deliberately avoiding a permanently-exported foot-gun from a
  half-finished config). The new `handleExportSchedule()` only acts on
  *state transitions* (compares the schedule's desired state against
  `zfs.IsImported(pool)`), so it is safe to call on every `pool-rescan` tick
  (default 60s -- window boundaries only need minute-level responsiveness,
  not the faster `interval` tick). Every action goes through the existing
  `execSleepPool`/`execWakePool` functions unchanged, so it automatically
  gets the exact same guards as a manual `sleeppool`/`wakepool` call: the
  replication-in-flight check (including the new rc5 Windows guard), the
  boot-pool refusal, the never-sleep exclude set, and VM pause/resume (`+vm`
  applied automatically whenever `vm-mode != off`, matching manual pool
  actions -- no separate flag). `force` is always `false` for scheduled
  actions, same as a manual call without `--force`.
- **New `MinWindow` type** (minute-of-day, 0-1439, HH:MM granularity, same
  midnight-crossing convention as the existing hour-granularity `Window`)
  backs both new fields, since hour granularity was too coarse for a forced
  export schedule. New config-level validation (`validateConfig`, run after
  every parse regardless of which frontend wrote the file, not only
  csweb-gui's own Settings form): overlapping windows within one list are
  rejected (midnight-crossing windows are split into their two segments
  before the pairwise check), and any pool referenced by `export-pools` or
  `pool-window` that is not also listed in `pools` is rejected.
- Config key syntax: `export-pools = pool1,pool2`; `export-timetable =
  HH:MM-HH:MM,HH:MM-HH:MM`; `pool-window =
  poolA:HH:MM-HH:MM,HH:MM-HH:MM;poolB:HH:MM-HH:MM` (`;` separates per-pool
  entries, `:` separates the pool name from its window list, `,` separates
  multiple windows within one list -- consistent with the existing
  comma-list convention used elsewhere in the config).
- New `config_test.go` covers `MinWindow.Active` (including midnight
  crossing), HH:MM and pool-window parsing/formatting round-trips, the two
  new validation rules, and end-to-end `parseConfig` of the three new keys.

## v1.1.0-rc5 (2026-08-20) — Release Candidate

Real behavior change (Windows only):

- **Windows replication guard implemented.** `internal/replcheck` previously
  always reported "no replication running" on Windows, on the assumption
  that OpenZFS on Windows is a storage target only and never runs `zfs
  send`/`receive` locally. That assumption doesn't hold for napp-it CS's own
  replication feature, which can run `zfs send`/`receive` directly on a
  Windows member -- so the replication gate (`sleepnow`, `sleeppool`
  including `--export`, `export-now`, and the daemon's idle loop all refuse
  to act while it reports active) provided **no protection at all** on
  Windows until now. Fixed: Windows now queries running processes via WMI
  (`Get-WmiObject Win32_Process`, matching this project's existing
  PowerShell-invocation pattern), filtered to `zfs.exe` processes whose
  command line contains `send`/`receive`/`recv` -- the same
  which-command-is-this filtering `pgrep -f "[z]fs send"` etc. already does
  on Unix. To avoid spawning a `powershell.exe` process on every daemon tick
  (default `interval`: 5s), the Windows result is cached for 30s and
  refreshed lazily; every one-shot invocation (`sleepnow`/`sleeppool`/
  `export-now`) still always starts as a fresh process with an empty cache,
  so those always see a real, uncached check -- only the long-running daemon
  loop benefits from (and needs) the caching. In practice this matters most
  for backup-pool export (`sleeppool --export`/`export-now`, including a
  scheduled forced export): an active pool's own idle/`verify-idle`
  requirement already tends to keep it from sleeping while replication I/O
  is ongoing, but export is a directed action that does not wait for idle,
  so this guard is its main protection on Windows.

## v1.1.0-rc4 (2026-08-20) — Release Candidate

Documentation/consistency fixes found while auditing the README against the
actual Go source -- no daemon/CLI behavior changes:

- **Version bump:** `main.go`'s hardcoded fallback `version` and `--help`
  banner text bumped from `1.1.0-rc3` to `1.1.0-rc4` (release binaries
  always get the correct string via the release workflow's `-ldflags -X
  main.version=...`; this keeps a plain local `go build .` honest too).
- **README corrected to match the actual config/CLI behavior:**
  - `disks` documented as the canonical config key (matches what
    `marshalConfig` actually writes and what `main.go`'s own usage text
    uses); `hd` is still accepted when reading a config file (legacy
    alias in `parseConfig`), now documented as such instead of as the
    primary name. Clarified that `disks` lists free/standalone disks
    managed *in addition to* disks resolved from `pools`.
  - `state-dir`/`log-file`/`pid-file` defaults documented as
    OS-conditional: Unix-like platforms default to `/var/run/cs-sleeper`
    and `/var/log/cs-sleeper.log`; Windows (no `/var/run`) falls back to
    `%TEMP%\cs-sleeper` and `%TEMP%\cs-sleeper\cs-sleeper.log`
    (`defaultStateDir`/`defaultLogFile` in `config.go`). The README
    previously stated the Unix-only defaults unconditionally.
  - Added a napp-it/csweb-gui integration note describing the
    active/backup pool categorization UI (a csweb-gui-side
    `_cfg/cs-sleeper.pooltype` file, never read by cs-sleeper itself) and
    that csweb-gui overrides `state-dir`/`log-file`/`pid-file` to its own
    `tmp/` folder rather than relying on the built-in defaults above.

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

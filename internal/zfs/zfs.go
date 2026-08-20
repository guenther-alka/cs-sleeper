// Package zfs wraps the zpool commands needed for pool safety checks and
// guarded export/import.
package zfs

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/sysio"
	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

// illumosDeviceRe matches illumos/Solaris short logical disk names as
// returned by `zpool status -P` on that platform (unlike Linux/FreeBSD/
// macOS, these are NOT "/"-prefixed): "c8d0", "c2t1d0", optionally with a
// slice suffix ("c8d0s0", "c2t1d0s2"), and with a hex/WWN-style target id
// ("c1t00253859019E709Dd0"). Confirmed live against a real illumos host:
// `zpool status -P b1` returned leaf lines "c8d0s0", "c9d0s0", "c10d0s0" with
// no "/" anywhere -- the pre-existing fields[0]-contains-"/" heuristic below
// silently skipped every one of them, so DisksOfPool() returned zero disks
// whenever the "-P" (no "-L") fallback path was used on illumos.
var illumosDeviceRe = regexp.MustCompile(`(?i)^c[0-9]+(t[0-9a-f]+)?d[0-9]+(s[0-9]+)?$`)

// cmdTimeout bounds every zpool invocation. zpool import/export on a large or
// degraded pool can legitimately take a while, but an unbounded call would
// let one unresponsive device wedge the whole daemon loop indefinitely.
const cmdTimeout = 90 * time.Second

// zpoolPath resolves the zpool executable, preferring well-known absolute
// install locations over a PATH search -- cs-sleeper normally runs as
// root/Administrator, where PATH-only resolution risks executing a
// look-alike binary planted earlier in PATH.
func zpoolPath() string {
	return xpath.Resolve("zpool",
		"/sbin/zpool",
		"/usr/sbin/zpool",
		"/usr/local/sbin/zpool",
		"/usr/local/bin/zpool",
		"/usr/local/zfs/bin/zpool",
		`C:\Program Files\OpenZFS On Windows\zpool.exe`,
	)
}

func zpoolOutput(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, zpoolPath(), args...).Output()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("zpool %s: timed out after %s", strings.Join(args, " "), cmdTimeout)
	}
	return out, err
}

// Pools returns the names of imported pools.
func Pools() ([]string, error) {
	out, err := zpoolOutput("list", "-H", "-o", "name")
	if err != nil {
		return nil, err
	}
	return splitLines(string(out)), nil
}

// IsImported reports whether pool is currently imported.
func IsImported(pool string) (bool, error) {
	ps, err := Pools()
	if err != nil {
		return false, err
	}
	for _, p := range ps {
		if p == pool {
			return true, nil
		}
	}
	return false, nil
}

// Export exports pool (optionally forced).
func Export(pool string, force bool) (string, error) {
	if !sysio.Valid(pool) {
		return "", fmt.Errorf("invalid pool name %q", pool)
	}
	args := []string{"export"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, pool)
	return run(args...)
}

// Import imports pool (optionally forced).
func Import(pool string, force bool) (string, error) {
	if !sysio.Valid(pool) {
		return "", fmt.Errorf("invalid pool name %q", pool)
	}
	args := []string{"import"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, pool)
	return run(args...)
}

// Status returns human-readable `zpool status -P` output for pool.
func Status(pool string) (string, error) {
	if !sysio.Valid(pool) {
		return "", fmt.Errorf("invalid pool name %q", pool)
	}
	return run("status", "-P", pool)
}

func run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, zpoolPath(), args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("zpool %s: timed out after %s", strings.Join(args, " "), cmdTimeout)
	}
	return string(out), err
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// preferPlainPathFirst reports whether `zpool status -P` (short cXtYdZ-style
// device names) should be tried before `-L` (resolved symlink target).
//
// On illumos/Solaris, `-L` is NOT unsupported (an earlier version of this
// comment claimed it was, based on an incorrect assumption) -- it succeeds
// and returns the full OBP physical device path (e.g.
// "pci@0,0/pci-ide@1f,2/ide@1/cmdk@0,0:a"), comma-separated in summary
// output and *not* prefixed with "/dev/". That path form is useless for
// sysio.DevicePath() on illumos, which expects a short logical name
// (c2t1d0) and builds "/dev/rdsk/<name>s2" from it: fed the physical path
// instead, it produces a nonsensical, unopenable device path, which made
// every smartctl sleep/wake call fail silently (see sleeper.Engine's
// verified-state fix in sleeper.go). So on illumos/Solaris, prefer the
// short "-P" form and only fall back to "-L" if "-P" itself fails.
//
// On the other platforms `-L` output is already "/dev/..."-prefixed and
// safe for sysio.DevicePath(), so it stays preferred there (it typically
// resolves symlinks/by-id paths to the real block device).
func preferPlainPathFirst() bool {
	return runtime.GOOS == "illumos" || runtime.GOOS == "solaris"
}

// zpoolStatusForDisks returns raw `zpool status` output for pool, trying the
// flag order best suited to the host platform (see preferPlainPathFirst)
// and falling back to the other flag if the first attempt errors.
func zpoolStatusForDisks(pool string) (string, error) {
	first, second := []string{"status", "-P", "-L", pool}, []string{"status", "-P", pool}
	if preferPlainPathFirst() {
		first, second = second, first
	}
	out, err := zpoolOutput(first...)
	if err != nil {
		if out2, err2 := zpoolOutput(second...); err2 == nil {
			return string(out2), nil
		}
		return "", err
	}
	return string(out), nil
}

// DisksOfPool returns the normalized device names of the data and hot-spare
// disks of pool (everything except SLOG/L2ARC/special/dedup flash devices).
// Flash devices are excluded so they are never spun down. See
// zpoolStatusForDisks for the platform-dependent flag order.
func DisksOfPool(pool string) ([]string, error) {
	if !sysio.Valid(pool) {
		return nil, fmt.Errorf("invalid pool name %q", pool)
	}
	out, err := zpoolStatusForDisks(pool)
	if err != nil {
		return nil, err
	}
	data, _ := parseZpoolStatus(out)
	return data, nil
}

// DisksOfPools returns the union of member disks for all pools, deduplicated.
// Pools that cannot be resolved (e.g. no zpool binary) are skipped.
func DisksOfPools(pools []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range pools {
		for _, d := range DisksOfPoolSafe(p) {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

// DisksOfPoolSafe is DisksOfPool but never returns an error.
func DisksOfPoolSafe(pool string) []string {
	ds, err := DisksOfPool(pool)
	if err != nil {
		return nil
	}
	return ds
}

// NeverSleepDisks returns the SLOG (logs), L2ARC (cache), special and dedup
// class disks of pool -- flash devices that must never be spun down. It never
// returns an error; unsupported hosts yield nil.
func NeverSleepDisks(pool string) []string {
	if !sysio.Valid(pool) {
		return nil
	}
	out, err := zpoolStatusForDisks(pool)
	if err != nil {
		return nil
	}
	_, flash := parseZpoolStatus(out)
	return flash
}

// BootPool returns the name of the pool whose `bootfs` property is set (the
// pool that holds the OS root filesystem), or "" if none is found.
func BootPool() (string, error) {
	out, err := zpoolOutput("list", "-H", "-o", "name,bootfs")
	if err != nil {
		return "", err
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		if fields[1] != "-" && fields[1] != "" {
			return fields[0], nil
		}
	}
	return "", sc.Err()
}

// parseZpoolStatus extracts device names from `zpool status -P` output,
// splitting them into data disks (data vdevs + hot spares) and flash disks
// (SLOG "logs", L2ARC "cache", "special" and "dedup" classes). Device lines
// are the leaf lines whose first column is either a full path (contains
// "/", the Linux/FreeBSD/macOS "-P"/"-L" form) or an illumos/Solaris short
// logical disk name (c8d0, c2t1d0, optionally sliced -- see illumosDeviceRe;
// that platform's "-P" output is never "/"-prefixed). A lone section
// keyword switches the current class.
func parseZpoolStatus(s string) (data, flash []string) {
	section := "data"
	dataSeen := map[string]bool{}
	flashSeen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) == 1 && !strings.Contains(fields[0], "/") {
			switch strings.ToLower(fields[0]) {
			case "logs", "log", "cache", "special", "dedup":
				section = "flash"
			case "spares", "spare":
				section = "data"
			}
			continue
		}
		if !strings.Contains(fields[0], "/") && !illumosDeviceRe.MatchString(fields[0]) {
			continue
		}
		n := sysio.Normalize(fields[0])
		if n == "" {
			continue
		}
		if section == "flash" {
			if !flashSeen[n] {
				flashSeen[n] = true
				flash = append(flash, n)
			}
		} else if !dataSeen[n] {
			dataSeen[n] = true
			data = append(data, n)
		}
	}
	return data, flash
}

// Package zfs wraps the zpool commands needed for pool safety checks and
// guarded export/import.
package zfs

import (
	"bufio"
	"os/exec"
	"strings"

	"github.com/guenther-alka/cs-sleeper/internal/sysio"
)

// Pools returns the names of imported pools.
func Pools() ([]string, error) {
	out, err := exec.Command("zpool", "list", "-H", "-o", "name").Output()
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
	args := []string{"export"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, pool)
	return run(args...)
}

// Import imports pool (optionally forced).
func Import(pool string, force bool) (string, error) {
	args := []string{"import"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, pool)
	return run(args...)
}

// Status returns human-readable `zpool status -P` output for pool.
func Status(pool string) (string, error) {
	return run("status", "-P", pool)
}

func run(args ...string) (string, error) {
	out, err := exec.Command("zpool", args...).CombinedOutput()
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

// DisksOfPool returns the normalized device names of the data and hot-spare
// disks of pool (everything except SLOG/L2ARC/special/dedup flash devices),
// resolved via `zpool status -P -L` (falling back to `-P` where -L is
// unsupported, e.g. illumos/Solaris). Flash devices are excluded so they are
// never spun down.
func DisksOfPool(pool string) ([]string, error) {
	out, err := exec.Command("zpool", "status", "-P", "-L", pool).Output()
	if err != nil {
		if out2, err2 := exec.Command("zpool", "status", "-P", pool).Output(); err2 == nil {
			data, _ := parseZpoolStatus(string(out2))
			return data, nil
		}
		return nil, err
	}
	data, _ := parseZpoolStatus(string(out))
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
	out, err := exec.Command("zpool", "status", "-P", "-L", pool).Output()
	if err != nil {
		if out2, err2 := exec.Command("zpool", "status", "-P", pool).Output(); err2 == nil {
			_, flash := parseZpoolStatus(string(out2))
			return flash
		}
		return nil
	}
	_, flash := parseZpoolStatus(string(out))
	return flash
}

// BootPool returns the name of the pool whose `bootfs` property is set (the
// pool that holds the OS root filesystem), or "" if none is found.
func BootPool() (string, error) {
	out, err := exec.Command("zpool", "list", "-H", "-o", "name,bootfs").Output()
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
// are the leaf lines whose first column is a full path (contains "/"); a lone
// section keyword switches the current class.
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
		if !strings.Contains(fields[0], "/") {
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

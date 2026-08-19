// Package zfs wraps the zpool commands needed for pool safety checks and
// guarded export/import.
package zfs

import (
	"os/exec"
	"strings"
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

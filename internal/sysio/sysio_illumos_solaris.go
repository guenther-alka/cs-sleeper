//go:build illumos || solaris

package sysio

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

const bootDiskCmdTimeout = 15 * time.Second

func iostatPath() string {
	return xpath.Resolve("iostat", "/usr/bin/iostat", "/usr/sbin/iostat", "/bin/iostat")
}

func dfPath() string {
	return xpath.Resolve("df", "/usr/bin/df", "/bin/df")
}

// NewReader returns the illumos/Solaris iostat-based reader.
func NewReader() Reader { return solarisReader{} }

type solarisReader struct{}

// DevicePath returns the raw disk path for a logical name (c2t1d0).
//
// FOUND LIVE cs_26.08.20 (Gea report: smartctl "Unable to detect device
// type" on member .203, disk daten1/c6t5000cca0bbe3ce1cd0): illumos'
// /dev/rdsk/ symlinks embed the disk's WWN in mixed/upper-case hex (e.g.
// c6t5000CCA0BBE3CE1Cd0), but every managed-disk name flowing through this
// package has already gone through sysio.Normalize(), which lower-cases it
// for cross-platform comparison. Building the path directly from that
// lower-cased name produces a filename that doesn't exist on illumos'
// case-sensitive /dev tree, so smartctl can't even open the device --
// confirmed live: /dev/rdsk/c6t5000cca0bbe3ce1cd0s2 fails to open ("Unable
// to detect device type"), while the real /dev/rdsk/c6t5000CCA0BBE3CE1Cd0s2
// works (smartctl -a succeeds, smartctl -s standby,now succeeds). Also
// confirmed `iostat -xn` itself reports the correct upper-case name --
// Normalize() is what introduces the mismatch, not the kstat/iostat source.
// resolveDeviceCase looks up the actual on-disk entry via a case-insensitive
// scan of /dev/rdsk so the smartctl invocation always uses the real casing;
// Normalize() itself is left untouched since its lower-cased form is still
// the right comparison key for exclude-lists/maps elsewhere in this project.
func DevicePath(name string) string {
	if strings.HasPrefix(name, "/dev/rdsk/") || strings.HasPrefix(name, "/dev/dsk/") {
		return name
	}
	if real := resolveDeviceCase(name); real != "" {
		name = real
	}
	return "/dev/rdsk/" + name + "s2"
}

// resolveDeviceCase returns the actual-case /dev/rdsk entry name (without
// the s2 suffix) matching name case-insensitively, or "" if /dev/rdsk can't
// be read or no entry matches (caller falls back to the given name as-is).
func resolveDeviceCase(name string) string {
	entries, err := os.ReadDir("/dev/rdsk")
	if err != nil {
		return ""
	}
	want := strings.ToLower(name) + "s2"
	for _, e := range entries {
		if strings.ToLower(e.Name()) == want {
			return strings.TrimSuffix(e.Name(), "s2")
		}
	}
	return ""
}

// Sample runs `iostat -xn <n> 2` and reports the second (interval-rate)
// report; Active marks devices that showed I/O in the window.
func (solarisReader) Sample(window int) ([]Counter, error) {
	if window < 1 {
		window = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(window)*time.Second*3+bootDiskCmdTimeout)
	out, err := exec.CommandContext(ctx, iostatPath(), "-xn", strconv.Itoa(window), "2").Output()
	cancel()
	if err != nil {
		return nil, err
	}
	return parseSolarisIostat(string(out)), nil
}

// parseSolarisIostat parses `iostat -xn` output where the device name is the
// last column and r/s, w/s are the first two columns.
func parseSolarisIostat(s string) []Counter {
	var out []Counter
	seen := map[string]int{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.Contains(line, "device") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		dev := Normalize(fields[len(fields)-1])
		if !looksDevice(dev) {
			continue
		}
		r, _ := strconv.ParseFloat(fields[0], 64)
		w, _ := strconv.ParseFloat(fields[1], 64)
		c := Counter{Device: dev, Active: r > 0 || w > 0}
		if i, ok := seen[dev]; ok {
			out[i] = c // keep only the last (interval) report
		} else {
			seen[dev] = len(out)
			out = append(out, c)
		}
	}
	return out
}

// BootDisks returns the whole physical disk that holds the root filesystem,
// resolved via `df /`. A ZFS root (rpool/...) yields nil; that case is covered
// by the pool-based fallback in the daemon.
func BootDisks() []string {
	ctx, cancel := context.WithTimeout(context.Background(), bootDiskCmdTimeout)
	out, err := exec.CommandContext(ctx, dfPath(), "/").Output()
	cancel()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return nil
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) == 0 {
		return nil
	}
	src := fields[0]
	if !strings.HasPrefix(src, "/dev/dsk/") && !strings.HasPrefix(src, "/dev/rdsk/") {
		return nil
	}
	if n := Normalize(src); n != "" {
		return []string{n}
	}
	return nil
}

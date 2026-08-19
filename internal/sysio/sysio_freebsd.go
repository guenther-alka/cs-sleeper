//go:build freebsd

package sysio

import (
	"bufio"
	"context"
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
	return xpath.Resolve("df", "/bin/df", "/usr/bin/df")
}

// NewReader returns the FreeBSD iostat-based reader.
func NewReader() Reader { return freebsdReader{} }

type freebsdReader struct{}

// DevicePath returns the device path for a short name (ada0, da0).
func DevicePath(name string) string {
	if strings.HasPrefix(name, "/dev/") {
		return name
	}
	return "/dev/" + name
}

// Sample runs `iostat -x -w <n> -c 2` and reports the second (interval-rate)
// report; Active marks devices that showed I/O in the window.
func (freebsdReader) Sample(window int) ([]Counter, error) {
	if window < 1 {
		window = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(window)*time.Second*3+bootDiskCmdTimeout)
	out, err := exec.CommandContext(ctx, iostatPath(), "-x", "-w", strconv.Itoa(window), "-c", "2").Output()
	cancel()
	if err != nil {
		return nil, err
	}
	return parseFreeBSDIostat(string(out)), nil
}

// parseFreeBSDIostat parses `iostat -x` output where the device name is the
// first column and r/s, w/s are the next two columns.
func parseFreeBSDIostat(s string) []Counter {
	var out []Counter
	seen := map[string]int{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "device") ||
			strings.HasPrefix(line, "extended") || strings.HasPrefix(line, "tty") ||
			strings.HasPrefix(line, "cpu") || strings.HasPrefix(line, "load") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		dev := Normalize(fields[0])
		if !looksDevice(dev) {
			continue
		}
		r, _ := strconv.ParseFloat(fields[1], 64)
		w, _ := strconv.ParseFloat(fields[2], 64)
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
// resolved via `df /`. A ZFS root (zroot/...) yields nil; that case is covered
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
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/dev/") {
		return nil
	}
	if n := Normalize(fields[0]); n != "" {
		return []string{n}
	}
	return nil
}

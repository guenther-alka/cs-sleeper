//go:build illumos || solaris

package sysio

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
)

// NewReader returns the illumos/Solaris iostat-based reader.
func NewReader() Reader { return solarisReader{} }

type solarisReader struct{}

// DevicePath returns the raw disk path for a logical name (c2t1d0).
func DevicePath(name string) string {
	if strings.HasPrefix(name, "/dev/rdsk/") || strings.HasPrefix(name, "/dev/dsk/") {
		return name
	}
	return "/dev/rdsk/" + name + "s2"
}

// Sample runs `iostat -xn <n> 2` and reports the second (interval-rate)
// report; Active marks devices that showed I/O in the window.
func (solarisReader) Sample(window int) ([]Counter, error) {
	if window < 1 {
		window = 1
	}
	out, err := exec.Command("iostat", "-xn", strconv.Itoa(window), "2").Output()
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
	out, err := exec.Command("df", "/").Output()
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

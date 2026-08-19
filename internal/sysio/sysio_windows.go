//go:build windows

package sysio

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
)

// NewReader returns the Windows performance-counter reader.
func NewReader() Reader { return windowsReader{} }

type windowsReader struct{}

// DevicePath returns the device as-is (smartctl accepts PhysicalDriveN or the
// OpenZFS /dev/sdN alias on Windows).
func DevicePath(name string) string { return name }

// Sample samples PhysicalDisk(*) Disk Read/Write Bytes/sec over the window
// using PowerShell Get-Counter and reports the interval-rate sample.
func (windowsReader) Sample(window int) ([]Counter, error) {
	if window < 1 {
		window = 1
	}
	script := "Get-Counter '\\PhysicalDisk(*)\\Disk Read Bytes/sec','\\PhysicalDisk(*)\\Disk Write Bytes/sec' " +
		"-SampleInterval " + strconv.Itoa(window) + " -MaxSamples 2 | " +
		"Select-Object -ExpandProperty CounterSamples | ForEach-Object { " +
		"$_.InstanceName + '|' + ($_.Path -split '\\\\')[-1] + '|' + $_.CookedValue }"
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return nil, err
	}
	return parseWinCounters(string(out)), nil
}

// parseWinCounters parses "<instance>|<counter>|<value>" lines produced by the
// PowerShell snippet above; instance "0 c:" maps to PhysicalDisk0.
func parseWinCounters(s string) []Counter {
	type acc struct{ r, w float64 }
	m := map[string]*acc{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			continue
		}
		inst := strings.TrimSpace(parts[0])
		counter := strings.ToLower(parts[1])
		val, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)

		num := strings.TrimSpace(strings.SplitN(inst, " ", 2)[0])
		if num == "" || num == "_total" || num == "harddiskvolume" {
			continue
		}
		if _, err := strconv.Atoi(num); err != nil {
			continue
		}
		dev := "PhysicalDisk" + num
		a := m[dev]
		if a == nil {
			a = &acc{}
			m[dev] = a
		}
		if strings.Contains(counter, "read") {
			a.r = val
		} else if strings.Contains(counter, "write") {
			a.w = val
		}
	}
	out := make([]Counter, 0, len(m))
	for dev, a := range m {
		out = append(out, Counter{Device: dev, Active: a.r > 0 || a.w > 0})
	}
	return out
}

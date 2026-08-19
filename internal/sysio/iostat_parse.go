package sysio

import (
	"bufio"
	"strconv"
	"strings"
)

// parseDarwinIostat parses `iostat -d` output, where devices are laid out
// side by side and each device contributes three columns (KB/t, tps, MB/s).
// A device-name row precedes a "KB/t tps MB/s" header row, then a numeric data
// row follows; this repeats once per report. Only the last report per device
// is kept (the interval rate, not the since-boot average).
func parseDarwinIostat(s string) []Counter {
	var (
		out     []Counter
		devices []string
		seen    = map[string]int{}
		reset   bool
	)
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		switch {
		case isMacHeader(fields): // "KB/t tps MB/s" row(s)
			continue
		case isMacDeviceRow(fields): // "disk0 disk1 ..." row(s)
			if reset {
				devices = devices[:0]
				reset = false
			}
			devices = append(devices, fields...)
		case allNumeric(fields):
			if len(devices) == 0 {
				continue
			}
			for i := 0; i+2 < len(fields); i += 3 {
				idx := i / 3
				if idx >= len(devices) {
					break
				}
				tps, _ := strconv.ParseFloat(fields[i+1], 64)
				c := Counter{Device: devices[idx], Active: tps > 0}
				if j, ok := seen[devices[idx]]; ok {
					out[j] = c
				} else {
					seen[devices[idx]] = len(out)
					out = append(out, c)
				}
			}
			reset = true
		}
	}
	return out
}

func isMacHeader(fields []string) bool {
	return len(fields) > 0 && fields[0] == "KB/t"
}

func isMacDeviceRow(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if !isMacDisk(f) {
			return false
		}
	}
	return true
}

func isMacDisk(s string) bool {
	if len(s) <= 4 || !strings.HasPrefix(s, "disk") {
		return false
	}
	for _, r := range s[4:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func allNumeric(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if _, err := strconv.ParseFloat(f, 64); err != nil {
			return false
		}
	}
	return true
}

// Package sysio samples per-disk I/O activity for the host OS.
package sysio

import "strings"

// Counter is one disk's I/O activity at a point in time.
//
// Cumulative readers (Linux /proc/diskstats) populate ReadIO/WriteIO with
// totals; the caller detects activity by comparing successive samples.
// Rate-based readers (illumos/Solaris/FreeBSD/Windows iostat or performance
// counters) leave ReadIO/WriteIO at zero and set Active to true when I/O
// occurred during the sample window.
type Counter struct {
	Device  string
	ReadIO  uint64
	WriteIO uint64
	Active  bool
}

// Reader samples per-disk I/O. windowSeconds is the sampling window for
// rate-based readers; cumulative readers may ignore it.
type Reader interface {
	Sample(windowSeconds int) ([]Counter, error)
}

// Active reports whether the current sample shows I/O since the previous
// sample. It handles both cumulative (delta) and rate-based (Active) sources.
func Active(prev, cur Counter) bool {
	if cur.Active {
		return true
	}
	return cur.ReadIO > prev.ReadIO || cur.WriteIO > prev.WriteIO
}

// Normalize reduces a user-supplied device name to a comparable short name:
// it strips leading /dev/, /dev/rdsk/, /dev/dsk/, /devices/ and Windows
// \\.\ prefixes, maps PhysicalDriveN to PhysicalDiskN, lowercases the name
// and removes FreeBSD/Solaris slice/partition and Linux partition suffixes.
func Normalize(name string) string {
	name = strings.TrimSpace(name)
	for _, p := range []string{"/dev/rdsk/", "/dev/dsk/", "/devices/", "/dev/", `\\.\`} {
		if strings.HasPrefix(name, p) {
			name = strings.TrimPrefix(name, p)
			break
		}
	}
	name = strings.ToLower(name)
	if strings.HasPrefix(name, "physicaldrive") {
		name = "physicaldisk" + strings.TrimPrefix(name, "physicaldrive")
	}
	// FreeBSD/Solaris slice or partition suffix: ada0s1 -> ada0, da0p1 -> da0,
	// c2t1d0s2 -> c2t1d0, nvme0n1p1 -> nvme0n1.
	if len(name) > 2 {
		if c := name[len(name)-2]; (c == 's' || c == 'p') && isDigit(name[len(name)-1]) {
			name = name[:len(name)-2]
		}
	}
	// Linux partition suffix for plain sd/vd/hd/xvd disks: sda1 -> sda.
	if (strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "vd") ||
		strings.HasPrefix(name, "hd") || strings.HasPrefix(name, "xvd")) && len(name) > 2 {
		if isDigit(name[len(name)-1]) {
			name = strings.TrimRight(name, "0123456789")
		}
	}
	return name
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// looksDevice reports whether a token looks like a disk device name (contains
// at least one letter and one digit, e.g. sda, ada0, c2t1d0, disk0).
func looksDevice(s string) bool {
	hasDigit, hasLetter := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			hasLetter = true
		}
	}
	return hasDigit && hasLetter
}

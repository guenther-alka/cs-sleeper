//go:build linux

package sysio

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// NewReader returns the Linux /proc/diskstats reader.
func NewReader() Reader { return linuxReader{} }

type linuxReader struct{}

// DevicePath returns the block-device path for a short name.
func DevicePath(name string) string {
	if strings.HasPrefix(name, "/dev/") {
		return name
	}
	return "/dev/" + name
}

// Sample reads cumulative per-disk counters from /proc/diskstats; the window
// argument is ignored because the counters are cumulative.
func (linuxReader) Sample(int) ([]Counter, error) {
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Counter
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 {
			continue
		}
		name := fields[2]
		if !wholeLinux(name) {
			continue
		}
		reads, _ := strconv.ParseUint(fields[3], 10, 64)
		writes, _ := strconv.ParseUint(fields[7], 10, 64)
		out = append(out, Counter{Device: name, ReadIO: reads, WriteIO: writes})
	}
	return out, sc.Err()
}

// wholeLinux reports whether a diskstats device name is a whole physical disk
// (rather than a partition, loop device, zvol, RAID or other pseudo device).
func wholeLinux(name string) bool {
	switch {
	case strings.HasPrefix(name, "loop"), strings.HasPrefix(name, "ram"),
		strings.HasPrefix(name, "zram"), strings.HasPrefix(name, "sr"),
		strings.HasPrefix(name, "fd"), strings.HasPrefix(name, "md"),
		strings.HasPrefix(name, "dm-"), strings.HasPrefix(name, "zd"),
		strings.HasPrefix(name, "nbd"), strings.HasPrefix(name, "rbd"):
		return false
	case strings.HasPrefix(name, "nvme"):
		return !strings.Contains(name, "p")
	case strings.HasPrefix(name, "mmcblk"):
		return !strings.Contains(strings.TrimPrefix(name, "mmcblk"), "p")
	case strings.HasPrefix(name, "sd"), strings.HasPrefix(name, "vd"),
		strings.HasPrefix(name, "hd"), strings.HasPrefix(name, "xvd"):
		return !hasDigitSuffix(name)
	}
	return false
}

func hasDigitSuffix(s string) bool {
	if s == "" {
		return false
	}
	c := s[len(s)-1]
	return c >= '0' && c <= '9'
}

// BootDisks returns the whole physical disk that holds the root filesystem,
// resolved via findmnt + lsblk. It returns nil when / is not on a plain block
// device (e.g. a ZFS or LVM root); that case is covered by the pool-based
// fallback in the daemon.
func BootDisks() []string {
	out, err := exec.Command("findmnt", "-n", "-o", "SOURCE", "/").Output()
	if err != nil {
		return nil
	}
	src := strings.TrimSpace(string(out))
	if src == "" || !strings.HasPrefix(src, "/dev/") {
		return nil
	}
	dev := strings.TrimPrefix(src, "/dev/")
	if pk, err := exec.Command("lsblk", "-ndo", "pkname", "/dev/"+dev).Output(); err == nil {
		if p := Normalize(strings.TrimSpace(string(pk))); p != "" {
			return []string{p}
		}
	}
	if n := Normalize(dev); n != "" {
		return []string{n}
	}
	return nil
}

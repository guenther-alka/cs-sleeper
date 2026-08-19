//go:build darwin

package sysio

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

const bootDiskCmdTimeout = 15 * time.Second

func iostatPath() string {
	return xpath.Resolve("iostat", "/usr/sbin/iostat", "/usr/bin/iostat")
}

func diskutilPath() string {
	return xpath.Resolve("diskutil", "/usr/sbin/diskutil")
}

// NewReader returns the macOS iostat-based reader.
func NewReader() Reader { return darwinReader{} }

type darwinReader struct{}

// DevicePath returns the block-device path for a short name (disk0).
func DevicePath(name string) string {
	if strings.HasPrefix(name, "/dev/") {
		return name
	}
	return "/dev/" + name
}

// Sample runs `iostat -d -w <n> -c 2` and reports the second (interval-rate)
// report; Active marks devices that showed I/O in the window.
func (darwinReader) Sample(window int) ([]Counter, error) {
	if window < 1 {
		window = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(window)*time.Second*3+bootDiskCmdTimeout)
	out, err := exec.CommandContext(ctx, iostatPath(), "-d", "-w", strconv.Itoa(window), "-c", "2").Output()
	cancel()
	if err != nil {
		return nil, err
	}
	return parseDarwinIostat(string(out)), nil
}

// BootDisks returns the whole physical disk(s) that hold the root filesystem,
// resolved via `diskutil info /`. On APFS the root volume is a synthesized
// container, so the physical store is used instead (e.g. /dev/disk3s5s1 ->
// disk2).
func BootDisks() []string {
	ctx, cancel := context.WithTimeout(context.Background(), bootDiskCmdTimeout)
	out, err := exec.CommandContext(ctx, diskutilPath(), "info", "/").Output()
	cancel()
	if err != nil {
		return nil
	}
	return parseDiskutilInfo(string(out))
}

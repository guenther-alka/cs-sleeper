//go:build darwin

package sysio

import "strings"

// NewReader returns a no-op reader for macOS.
func NewReader() Reader { return darwinReader{} }

type darwinReader struct{}

// DevicePath returns the device path for a short name (disk0).
func DevicePath(name string) string {
	if strings.HasPrefix(name, "/dev/") {
		return name
	}
	return "/dev/" + name
}

// Sample returns no devices: macOS is not a supported napp-it platform, so
// cs-sleeper only builds there for completeness. smartctl remains usable via
// the one-shot sleepnow/wakeupnow commands.
func (darwinReader) Sample(int) ([]Counter, error) { return nil, nil }

//go:build !linux && !freebsd && !illumos && !solaris && !windows && !darwin

package sysio

// NewReader returns a no-op reader for unsupported platforms.
func NewReader() Reader { return otherReader{} }

type otherReader struct{}

// DevicePath returns the device name as-is.
func DevicePath(name string) string { return name }

func (otherReader) Sample(int) ([]Counter, error) { return nil, nil }

// Package vm suspends/resumes (or shuts down/starts) virtual machines that
// live on a ZFS pool, so a pool can be slept without corrupting running VMs.
package vm

import (
	"fmt"
	"strings"
)

// Action is how a VM is put to sleep.
type Action int

const (
	ActionNone Action = iota
	ActionSuspend
	ActionShutdown
)

// VM is one discovered virtual machine.
type VM struct {
	ID     string
	Name   string
	Status string
}

// Backend implements the hypervisor-specific operations.
type Backend interface {
	// VMsOnPool returns the VMs whose disks live on the given ZFS pool.
	VMsOnPool(pool string) ([]VM, error)
	// Sleep pauses or shuts down a VM (depending on the configured action).
	Sleep(vmID string) error
	// Wake resumes or starts a VM.
	Wake(vmID string) error
}

// New returns a Backend for the configured mode string, or nil for "off".
// Supported modes: proxmox_suspend, proxmox_shutdown (and hyphenated variants).
func New(mode string) (Backend, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "off", "none":
		return nil, nil
	case "proxmox_suspend", "proxmox-suspend":
		return &proxmoxBackend{action: ActionSuspend}, nil
	case "proxmox_shutdown", "proxmox-shutdown":
		return &proxmoxBackend{action: ActionShutdown}, nil
	default:
		return nil, fmt.Errorf("unsupported vm-mode %q (want proxmox_suspend, proxmox_shutdown or off)", mode)
	}
}

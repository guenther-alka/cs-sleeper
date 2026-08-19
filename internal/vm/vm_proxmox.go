package vm

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

// cmdTimeoutShort bounds read-only qm calls (list, config); cmdTimeoutLong
// bounds state-changing calls (shutdown/suspend/start/resume), which may
// legitimately take longer for a graceful guest shutdown but must still not
// block the daemon forever if a VM hangs.
const (
	cmdTimeoutShort = 30 * time.Second
	cmdTimeoutLong  = 120 * time.Second
)

// qmPath resolves the Proxmox qm executable, preferring its well-known
// absolute install location over a PATH search (cs-sleeper runs as root).
func qmPath() string {
	return xpath.Resolve("qm", "/usr/sbin/qm", "/usr/bin/qm")
}

type proxmoxBackend struct {
	action Action
}

// VMsOnPool discovers Proxmox VMs whose disks are on the given ZFS pool by
// mapping storages (from /etc/pve/storage.cfg) to their backing pool and then
// inspecting each VM's disk configuration.
func (p *proxmoxBackend) VMsOnPool(pool string) ([]VM, error) {
	poolOfStorage, err := storagePools()
	if err != nil {
		return nil, err
	}
	out, err := qmOutput(cmdTimeoutShort, "list")
	if err != nil {
		return nil, fmt.Errorf("qm list: %w", err)
	}
	var vms []VM
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || !isNumeric(fields[0]) {
			continue
		}
		id := fields[0]
		cfgOut, err := qmOutput(cmdTimeoutShort, "config", id)
		if err != nil {
			continue
		}
		if !vmOnPool(string(cfgOut), pool, poolOfStorage) {
			continue
		}
		v := VM{ID: id, Name: vmName(string(cfgOut))}
		if len(fields) >= 3 {
			v.Status = fields[2]
		}
		vms = append(vms, v)
	}
	return vms, nil
}

func (p *proxmoxBackend) Sleep(vmID string) error {
	if !isNumeric(vmID) {
		return fmt.Errorf("invalid VM id %q", vmID)
	}
	if p.action == ActionShutdown {
		return run(cmdTimeoutLong, "shutdown", vmID)
	}
	return run(cmdTimeoutLong, "suspend", vmID, "--todisk")
}

func (p *proxmoxBackend) Wake(vmID string) error {
	if !isNumeric(vmID) {
		return fmt.Errorf("invalid VM id %q", vmID)
	}
	if p.action == ActionShutdown {
		return run(cmdTimeoutLong, "start", vmID)
	}
	return run(cmdTimeoutLong, "resume", vmID)
}

// storagePools parses /etc/pve/storage.cfg and returns storage -> pool.
func storagePools() (map[string]string, error) {
	b, err := os.ReadFile("/etc/pve/storage.cfg")
	if err != nil {
		return nil, fmt.Errorf("cannot read storage.cfg: %w", err)
	}
	m := map[string]string{}
	cur := ""
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ") {
			// section header: "zfspool: name" or "dir: name"
			if i := strings.Index(trimmed, ":"); i >= 0 {
				cur = strings.TrimSpace(trimmed[i+1:])
			}
			continue
		}
		if strings.HasPrefix(trimmed, "pool ") {
			ds := strings.TrimSpace(strings.TrimPrefix(trimmed, "pool "))
			// dataset "rpool/data" -> pool "rpool"
			if i := strings.Index(ds, "/"); i >= 0 {
				ds = ds[:i]
			}
			m[cur] = ds
		}
	}
	return m, nil
}

// vmOnPool reports whether the VM config references a storage backed by pool.
func vmOnPool(cfg, pool string, poolOfStorage map[string]string) bool {
	for _, storage := range diskStorages(cfg) {
		if poolOfStorage[storage] == pool {
			return true
		}
	}
	return false
}

// diskStorages extracts storage names from `qm config` volume references.
func diskStorages(cfg string) []string {
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(cfg))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, ":"); i >= 0 {
			val := strings.TrimSpace(line[i+1:])
			// volume reference looks like "storage:volid,..."
			if j := strings.Index(val, ":"); j >= 0 {
				storage := strings.TrimSpace(val[:j])
				if storage != "" && !seen[storage] {
					seen[storage] = true
					out = append(out, storage)
				}
			}
		}
	}
	return out
}

func vmName(cfg string) string {
	sc := bufio.NewScanner(strings.NewReader(cfg))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "name:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
	}
	return ""
}

// qmOutput runs `qm <args...>` with a timeout and returns its stdout.
func qmOutput(timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, qmPath(), args...).Output()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("qm %s: timed out after %s", strings.Join(args, " "), timeout)
	}
	return out, err
}

// run runs `qm <args...>` with a timeout, returning combined output on error.
func run(timeout time.Duration, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, qmPath(), args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("qm %s: timed out after %s", strings.Join(args, " "), timeout)
	}
	if err != nil {
		return fmt.Errorf("qm %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

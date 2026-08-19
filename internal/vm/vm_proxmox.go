package vm

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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
	out, err := exec.Command("qm", "list").Output()
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
		cfgOut, err := exec.Command("qm", "config", id).Output()
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
	if p.action == ActionShutdown {
		return run("qm", "shutdown", vmID)
	}
	return run("qm", "suspend", vmID, "--todisk")
}

func (p *proxmoxBackend) Wake(vmID string) error {
	if p.action == ActionShutdown {
		return run("qm", "start", vmID)
	}
	return run("qm", "resume", vmID)
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

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
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

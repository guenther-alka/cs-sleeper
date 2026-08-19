package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/replcheck"
	"github.com/guenther-alka/cs-sleeper/internal/sleeper"
	"github.com/guenther-alka/cs-sleeper/internal/vm"
	"github.com/guenther-alka/cs-sleeper/internal/zfs"
)

// execSleepPool puts a pool to sleep: optionally pauses/shuts down VMs on it,
// then either exports the pool (backup pools) or spins its disks to standby
// (active pools, pool stays imported).
func execSleepPool(cfg *Config, pool string, export, includeVM, force bool, logger *log.Logger) error {
	if err := guardBootPool(pool); err != nil {
		return err
	}
	if rep := replcheck.Check(); rep.Active {
		return fmt.Errorf("zfs send/receive in flight")
	}
	disks, err := zfs.DisksOfPool(pool)
	if err != nil {
		return fmt.Errorf("cannot resolve pool disks: %w", err)
	}
	// Defense in depth: also drop any disk that is separately protected
	// (exclude list, OS boot disk, or a boot-pool member resolved another
	// way), even though guardBootPool above already refuses the boot pool
	// itself by name.
	if protected := neverSleepSet(cfg); len(protected) > 0 {
		if filtered := excludeDevices(disks, protected); len(filtered) != len(disks) {
			logger.Printf("sleeppool %s: %d disk(s) protected, skipped", pool, len(disks)-len(filtered))
			disks = filtered
		}
	}
	if includeVM {
		ids, err := vmSleep(cfg, pool, logger)
		if err != nil {
			return err
		}
		_ = saveVMAffected(cfg, pool, ids)
	}
	toSleep := disks
	if export {
		if out, err := zfs.Export(pool, force); err != nil {
			return fmt.Errorf("zpool export failed: %v (%s)", err, strings.TrimSpace(out))
		}
		logger.Printf("sleeppool %s: exported", pool)
	} else {
		// Flush pending writes before standby so the disks stay asleep
		// instead of waking on the next ZFS transaction-group commit.
		if out, err := zfs.Sync(pool); err != nil {
			logger.Printf("sleeppool %s: sync failed: %v (%s)", pool, err, strings.TrimSpace(out))
		} else {
			logger.Printf("sleeppool %s: sync ok", pool)
		}
		// Re-verify: a write that landed in the ZFS RAM write cache during
		// the sync (or right after it) will hit the disks on the next txg
		// commit; sample again and only standby the disks that stayed idle.
		toSleep = reverifyIdle(disks, time.Duration(cfg.Interval)*time.Second)
		if skipped := len(disks) - len(toSleep); skipped > 0 {
			logger.Printf("sleeppool %s: re-verify: %d disk(s) active, skipped", pool, skipped)
		}
	}
	for _, e := range sleeper.SleepAll(toSleep, cfg.Parallel) {
		logger.Printf("sleeppool %s: %v", pool, e)
	}
	logger.Printf("sleeppool %s: %d disk(s) to standby", pool, len(toSleep))
	return nil
}

// execWakePool wakes a pool: imports it if necessary, spins its disks up and
// optionally resumes/starts VMs that were paused by a previous sleeppool.
func execWakePool(cfg *Config, pool string, includeVM, force bool, logger *log.Logger) error {
	imported, _ := zfs.IsImported(pool)
	if !imported {
		if out, err := zfs.Import(pool, force); err != nil {
			return fmt.Errorf("zpool import failed: %v (%s)", err, strings.TrimSpace(out))
		}
		logger.Printf("wakepool %s: imported", pool)
	}
	if disks, err := zfs.DisksOfPool(pool); err == nil {
		for _, e := range sleeper.WakeAll(disks, cfg.Parallel) {
			logger.Printf("wakepool %s: %v", pool, e)
		}
	} else {
		logger.Printf("wakepool %s: warning: cannot resolve disks: %v", pool, err)
	}
	if includeVM {
		if err := vmWake(cfg, pool, logger); err != nil {
			logger.Printf("wakepool %s: vm wake: %v", pool, err)
		}
	}
	logger.Printf("wakepool %s: done", pool)
	return nil
}

// vmSleep pauses or shuts down the VMs on pool (per cfg.VMMode) and returns the
// IDs it actually acted on.
func vmSleep(cfg *Config, pool string, logger *log.Logger) ([]string, error) {
	b, err := vm.New(cfg.VMMode)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, nil
	}
	vms, err := b.VMsOnPool(pool)
	if err != nil {
		return nil, err
	}
	if len(vms) == 0 {
		logger.Printf("sleeppool %s: no VMs found on pool", pool)
		return nil, nil
	}
	var ids []string
	for _, v := range vms {
		if err := b.Sleep(v.ID); err != nil {
			logger.Printf("sleeppool %s: vm %s: %v", pool, v.ID, err)
			continue
		}
		logger.Printf("sleeppool %s: vm %s slept", pool, v.ID)
		ids = append(ids, v.ID)
	}
	return ids, nil
}

// vmWake resumes or starts VMs. It prefers the IDs recorded by a previous
// sleeppool and falls back to discovering VMs on the pool.
func vmWake(cfg *Config, pool string, logger *log.Logger) error {
	b, err := vm.New(cfg.VMMode)
	if err != nil {
		return err
	}
	if b == nil {
		return nil
	}
	ids := loadVMAffected(cfg, pool)
	if len(ids) == 0 {
		vms, err := b.VMsOnPool(pool)
		if err != nil {
			return err
		}
		for _, v := range vms {
			ids = append(ids, v.ID)
		}
	}
	for _, id := range ids {
		if err := b.Wake(id); err != nil {
			logger.Printf("wakepool %s: vm %s: %v", pool, id, err)
			continue
		}
		logger.Printf("wakepool %s: vm %s woken", pool, id)
	}
	clearVMAffected(cfg, pool)
	return nil
}

func vmStateFile(cfg *Config) string { return filepath.Join(cfg.StateDir, "vmstate.json") }

func loadVMAffected(cfg *Config, pool string) []string {
	b, err := os.ReadFile(vmStateFile(cfg))
	if err != nil {
		return nil
	}
	var m map[string][]string
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m[pool]
}

func saveVMAffected(cfg *Config, pool string, ids []string) error {
	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		return err
	}
	m := map[string][]string{}
	if b, err := os.ReadFile(vmStateFile(cfg)); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	m[pool] = ids
	return writeJSONAtomic(vmStateFile(cfg), m)
}

func clearVMAffected(cfg *Config, pool string) {
	m := map[string][]string{}
	if b, err := os.ReadFile(vmStateFile(cfg)); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	delete(m, pool)
	_ = writeJSONAtomic(vmStateFile(cfg), m)
}

// Package sleeper implements the idle-detection engine and the smartctl
// sleep/wake primitives.
package sleeper

import "time"

// Options tune the engine.
type Options struct {
	Wait       time.Duration
	VerifyIdle int
	// TrackWake records disks that wake on access (previously sleeping and now
	// showing I/O). Disabled for the `manual` wake policy.
	TrackWake bool
	// AllowSleep returns false to block sleeping device at time t (e.g.
	// inside an activity window -- global, or a per-pool override).
	AllowSleep func(device string, t time.Time) bool
}

// Disk is a snapshot of one managed disk's state.
type Disk struct {
	Device     string
	Sleeping   bool
	LastActive time.Time
	LastWake   time.Time
	IdleSince  time.Time
}

type engineDisk struct {
	Disk
	verify int
}

// Engine tracks idle time per managed device.
type Engine struct {
	disks map[string]*engineDisk
	opt   Options
}

// NewEngine creates an engine for the given normalized device names.
func NewEngine(devices []string, opt Options) *Engine {
	if opt.VerifyIdle <= 0 {
		opt.VerifyIdle = 1
	}
	e := &Engine{disks: make(map[string]*engineDisk), opt: opt}
	for _, d := range devices {
		if d == "" {
			continue
		}
		e.disks[d] = &engineDisk{verify: opt.VerifyIdle}
	}
	return e
}

// Update feeds one sample (set of devices that showed I/O) and returns the
// devices that should now be put to sleep, plus — when TrackWake is enabled —
// the devices that woke on access (previously sleeping, now active).
func (e *Engine) Update(active map[string]bool, now time.Time) (sleep []string, woke []string) {
	for name, d := range e.disks {
		if active[name] {
			if e.opt.TrackWake && d.Sleeping {
				woke = append(woke, name)
				d.LastWake = now
			}
			d.LastActive = now
			d.IdleSince = time.Time{}
			d.verify = e.opt.VerifyIdle
			d.Sleeping = false
			continue
		}
		if d.IdleSince.IsZero() {
			d.IdleSince = now
		}
		if !e.opt.AllowSleep(name, now) {
			continue
		}
		if now.Sub(d.IdleSince) >= e.opt.Wait {
			if d.verify > 0 {
				d.verify--
			}
			if d.verify == 0 && !d.Sleeping {
				d.Sleeping = true
				sleep = append(sleep, name)
			}
		}
	}
	return sleep, woke
}

// Snapshot returns a copy of the current per-disk state.
func (e *Engine) Snapshot() []Disk {
	out := make([]Disk, 0, len(e.disks))
	for name, d := range e.disks {
		out = append(out, Disk{
			Device:     name,
			Sleeping:   d.Sleeping,
			LastActive: d.LastActive,
			LastWake:   d.LastWake,
			IdleSince:  d.IdleSince,
		})
	}
	return out
}

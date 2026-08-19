package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type taskKind string

const (
	taskSleepPool taskKind = "sleeppool"
	taskWakePool  taskKind = "wakepool"
)

// scheduledTask is a queued pool action to be run by the daemon at `At`.
type scheduledTask struct {
	ID        string    `json:"id"`
	Kind      taskKind  `json:"kind"`
	Pool      string    `json:"pool"`
	At        time.Time `json:"at"`
	Export    bool      `json:"export,omitempty"`
	IncludeVM bool      `json:"include_vm,omitempty"`
	Force     bool      `json:"force,omitempty"`
}

func scheduleFile(cfg *Config) string {
	return filepath.Join(cfg.StateDir, "schedule.json")
}

func loadSchedule(cfg *Config) ([]scheduledTask, error) {
	b, err := os.ReadFile(scheduleFile(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var tasks []scheduledTask
	if err := json.Unmarshal(b, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func saveSchedule(cfg *Config, tasks []scheduledTask) error {
	return writeJSONAtomic(scheduleFile(cfg), tasks)
}

// queueTask appends a scheduled task and persists the schedule.
func queueTask(cfg *Config, t scheduledTask) error {
	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		return err
	}
	tasks, err := loadSchedule(cfg)
	if err != nil {
		return err
	}
	t.ID = fmt.Sprintf("%s-%d", t.Kind, time.Now().UnixNano())
	tasks = append(tasks, t)
	return saveSchedule(cfg, tasks)
}

// runDueTasks executes and removes scheduled tasks whose time has come.
func runDueTasks(cfg *Config, logger *log.Logger) {
	tasks, err := loadSchedule(cfg)
	if err != nil {
		logger.Printf("warning: cannot read schedule: %v", err)
		return
	}
	if len(tasks) == 0 {
		return
	}
	now := time.Now()
	remaining := make([]scheduledTask, 0, len(tasks))
	for _, t := range tasks {
		if now.Before(t.At) {
			remaining = append(remaining, t)
			continue
		}
		if err := runTask(cfg, t, logger); err != nil {
			logger.Printf("scheduled %s %s failed: %v", t.Kind, t.Pool, err)
		}
	}
	if err := saveSchedule(cfg, remaining); err != nil {
		logger.Printf("warning: cannot write schedule: %v", err)
	}
}

func runTask(cfg *Config, t scheduledTask, logger *log.Logger) error {
	switch t.Kind {
	case taskSleepPool:
		return execSleepPool(cfg, t.Pool, t.Export, t.IncludeVM, t.Force, logger)
	case taskWakePool:
		return execWakePool(cfg, t.Pool, t.IncludeVM, t.Force, logger)
	default:
		return fmt.Errorf("unknown task kind %q", t.Kind)
	}
}

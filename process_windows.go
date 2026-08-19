//go:build windows

package main

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/guenther-alka/cs-sleeper/internal/xpath"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	tasklist := xpath.Resolve("tasklist", `C:\Windows\System32\tasklist.exe`)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, tasklist, "/FI", "PID eq "+strconv.Itoa(pid), "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), strconv.Itoa(pid))
}

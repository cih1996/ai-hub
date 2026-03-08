//go:build windows

package core

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

// buildCommand creates the exec.Cmd for Windows platforms.
func buildCommand(command string) *exec.Cmd {
	return exec.Command("cmd", "/C", command)
}

// setProcAttr is a no-op on Windows (no Setpgid equivalent needed).
func setProcAttr(cmd *exec.Cmd) {
	// No special process attributes needed on Windows
}

// killProcess kills a process on Windows using taskkill.
func killProcess(pid int) {
	// Try graceful kill first
	exec.Command("taskkill", "/PID", strconv.Itoa(pid)).Run()
	time.Sleep(2 * time.Second)
	if processAlive(pid) {
		// Force kill
		exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
	}
}

// processAlive checks if a process is alive on Windows.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds; use Signal(nil) which
	// calls OpenProcess internally — returns error if process doesn't exist.
	err = proc.Signal(os.Signal(nil))
	return err == nil
}

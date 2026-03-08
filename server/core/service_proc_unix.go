//go:build !windows

package core

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// buildCommand creates the exec.Cmd for Unix platforms.
func buildCommand(command string) *exec.Cmd {
	return exec.Command("sh", "-c", command)
}

// setProcAttr sets Unix-specific process attributes (detach process group).
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcess kills a process group on Unix (SIGTERM then SIGKILL).
func killProcess(pid int) {
	syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(2 * time.Second)
	if processAlive(pid) {
		syscall.Kill(-pid, syscall.SIGKILL)
	}
}

// processAlive checks if a process is alive using signal 0.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

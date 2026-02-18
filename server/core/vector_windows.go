//go:build windows

package core

import (
	"fmt"
	"os/exec"
	"syscall"
)

// newProcessGroupAttr returns SysProcAttr for Windows (no Setpgid)
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

// killProcessGroup kills the process tree on Windows using taskkill
func killProcessGroup(pid int, force bool) {
	args := []string{"/T", "/PID", fmt.Sprintf("%d", pid)}
	if force {
		args = append(args, "/F")
	}
	exec.Command("taskkill", args...).Run()
}

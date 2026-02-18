//go:build !windows

package core

import "syscall"

// newProcessGroupAttr returns SysProcAttr to create a new process group (Unix)
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends signal to the process group.
// force=false sends SIGTERM, force=true sends SIGKILL.
func killProcessGroup(pid int, force bool) {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	syscall.Kill(-pid, sig)
}

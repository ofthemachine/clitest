//go:build unix

package clitest

import (
	"os/exec"
	"syscall"
	"time"
)

// inGroup runs cmd in a process group of its own, which a timeout kills
// whole (the shell and everything it started).
func inGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	// Wait returns this long after a kill even if something still holds
	// the case's output open.
	cmd.WaitDelay = 5 * time.Second
}

// endGroup kills what is left of cmd's process group once it has ended.
func endGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

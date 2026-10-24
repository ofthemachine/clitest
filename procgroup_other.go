//go:build !unix

package clitest

import (
	"os/exec"
	"time"
)

// inGroup: without process groups, a timeout kills act.sh itself.
func inGroup(cmd *exec.Cmd) { cmd.WaitDelay = 5 * time.Second }

func endGroup(*exec.Cmd) {}

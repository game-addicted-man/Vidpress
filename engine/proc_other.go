//go:build !windows

package engine

import "os/exec"

// NullDevice is the path ffmpeg writes to when the output is discarded.
func NullDevice() string { return "/dev/null" }

func setProcAttr(cmd *exec.Cmd, lowPriority bool) {}

func attachToJob(cmd *exec.Cmd) {}

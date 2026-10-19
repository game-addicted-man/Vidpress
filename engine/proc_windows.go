//go:build windows

package engine

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	createNoWindow         = 0x08000000
	belowNormalPriorityCls = 0x00004000
)

// NullDevice is the path ffmpeg writes to when the output is discarded.
func NullDevice() string { return "NUL" }

// setProcAttr hides the console window of child processes and optionally
// lowers their priority so the PC stays responsive while encoding.
func setProcAttr(cmd *exec.Cmd, lowPriority bool) {
	flags := uint32(createNoWindow)
	if lowPriority {
		flags |= belowNormalPriorityCls
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
}

var (
	jobOnce   sync.Once
	jobHandle windows.Handle
)

// attachToJob puts the child into a job object that is closed together with
// VidPress, so ffmpeg never keeps running in the background after the app
// exits or crashes.
func attachToJob(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(h)
			return
		}
		jobHandle = h
	})
	if jobHandle == 0 {
		return
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	_ = windows.AssignProcessToJobObject(jobHandle, ph)
	windows.CloseHandle(ph)
}

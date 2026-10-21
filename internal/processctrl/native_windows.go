//go:build windows

package processctrl

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

type control struct {
	job windows.Handle
	pid uint32
}

func prepare(cmd *exec.Cmd, _ time.Duration) (*control, error) {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, e
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	hidden := cmd.SysProcAttr != nil && cmd.SysProcAttr.HideWindow
	flags := uint32(windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED)
	if hidden {
		flags |= windows.CREATE_NO_WINDOW
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: hidden}
	return &control{job: job}, nil
}
func (c *control) attach(cmd *exec.Cmd) error {
	c.pid = uint32(cmd.Process.Pid)
	h, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, c.pid)
	if e != nil {
		return e
	}
	defer windows.CloseHandle(h)
	if e = windows.AssignProcessToJobObject(c.job, h); e != nil {
		return e
	}
	// Resume only after job assignment, so a fast child cannot escape the job.
	status, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess").Call(uintptr(h))
	if status != 0 {
		return errors.New("owned child resume failed")
	}
	return nil
}
func (c *control) graceful(_ *exec.Cmd, _ os.Signal) {
	windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, c.pid)
}
func (c *control) kill() { windows.TerminateJobObject(c.job, 1) }
func (c *control) finish(grace time.Duration) {
	c.kill()
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		var info struct {
			TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
			TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
		}
		e := windows.QueryInformationJobObject(c.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
		if e != nil || info.ActiveProcesses == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func (c *control) close() { windows.CloseHandle(c.job) }

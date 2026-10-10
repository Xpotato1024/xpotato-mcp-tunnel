//go:build linux

package processctrl

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

type control struct{ pid int }

func prepare(cmd *exec.Cmd) (*control, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &control{}, nil
}
func (c *control) attach(cmd *exec.Cmd) error        { c.pid = cmd.Process.Pid; return nil }
func (c *control) graceful(_ *exec.Cmd, _ os.Signal) { syscall.Kill(-c.pid, syscall.SIGTERM) }
func (c *control) kill() {
	if c.pid > 0 {
		syscall.Kill(-c.pid, syscall.SIGKILL)
	}
}
func (c *control) finish(grace time.Duration) {
	c.kill()
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if syscall.Kill(-c.pid, 0) != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func (c *control) close() {}

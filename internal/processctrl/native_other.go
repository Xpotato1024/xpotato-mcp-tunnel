//go:build !windows && !linux && !darwin

package processctrl

import (
	"errors"
	"os"
	"os/exec"
	"time"
)

type control struct{}

func prepare(*exec.Cmd, time.Duration) (*control, error) {
	return nil, errors.New("unsupported child lifecycle")
}
func (*control) attach(*exec.Cmd) error        { return nil }
func (*control) graceful(*exec.Cmd, os.Signal) {}
func (*control) kill()                         {}
func (*control) finish(time.Duration)          {}
func (*control) close()                        {}

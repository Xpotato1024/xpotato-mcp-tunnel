// Package processctrl confines cleanup to a job/process group created for this
// launch. It never enumerates arbitrary processes or reads process arguments.
package processctrl

import (
	"os"
	"os/exec"
	"time"
)

func Run(cmd *exec.Cmd, signals <-chan os.Signal, started func(), grace time.Duration) (int, error) {
	control, e := prepare(cmd, grace)
	if e != nil {
		return 1, e
	}
	defer control.close()
	if e = cmd.Start(); e != nil {
		return 1, e
	}
	if e = control.attach(cmd); e != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return 1, e
	}
	if started != nil {
		started()
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-wait:
	case sig := <-signals:
		control.graceful(cmd, sig)
		select {
		case waitErr = <-wait:
		case <-time.After(grace):
			control.kill()
			waitErr = <-wait
		}
	}
	// A leader exiting successfully must not leave its owned descendants behind.
	control.finish(grace)
	if waitErr != nil {
		if exit, ok := waitErr.(*exec.ExitError); ok {
			return exit.ExitCode(), nil
		}
		return 1, waitErr
	}
	return 0, nil
}

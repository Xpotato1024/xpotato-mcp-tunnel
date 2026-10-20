// Package processctrl confines cleanup to a job/process group created for this
// launch. It never enumerates arbitrary processes or reads process arguments.
package processctrl

import (
	"context"
	"os"
	"os/exec"
	"time"
)

func RunContext(ctx context.Context, cmd *exec.Cmd, started func(), grace time.Duration) (int, error) {
	if e := ctx.Err(); e != nil {
		return 1, e
	}
	signals := make(chan os.Signal, 1)
	done, joined := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			signals <- os.Interrupt
		case <-done:
		}
	}()
	code, e := Run(cmd, signals, started, grace)
	close(done)
	<-joined
	if ctx.Err() != nil {
		return code, ctx.Err()
	}
	return code, e
}

func Run(cmd *exec.Cmd, signals <-chan os.Signal, started func(), grace time.Duration) (int, error) {
	return RunWithStatus(cmd, signals, started, grace, nil)
}

// Status is observational only; the same owned-process cleanup path is used.
func RunWithStatus(cmd *exec.Cmd, signals <-chan os.Signal, started func(), grace time.Duration, status func(string)) (int, error) {
	report := func(state string) {
		if status != nil {
			status(state)
		}
	}
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
	report("running")
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-wait:
	case sig := <-signals:
		report("stopping")
		control.graceful(cmd, sig)
		select {
		case waitErr = <-wait:
		case <-time.After(grace):
			control.kill()
			waitErr = <-wait
		}
	}
	// A leader exiting successfully must not leave its owned descendants behind.
	report("cleanup")
	control.finish(grace)
	report("stopped")
	if waitErr != nil {
		if exit, ok := waitErr.(*exec.ExitError); ok {
			return exit.ExitCode(), nil
		}
		return 1, waitErr
	}
	return 0, nil
}

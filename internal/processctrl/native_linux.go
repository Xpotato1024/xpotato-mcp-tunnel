//go:build linux

package processctrl

import (
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/lockfile"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const guardianArgument = "--xpotato-owned-linux-guardian-v1"

type control struct {
	read, write *os.File
	inherited   []*os.File
}

func prepare(cmd *exec.Cmd, grace time.Duration) (*control, error) {
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	locks, e := lockfile.InheritHeld()
	if e != nil {
		return nil, e
	}
	read, write, e := os.Pipe()
	if e != nil {
		for _, f := range locks {
			f.Close()
		}
		return nil, e
	}
	c := &control{read: read, write: write, inherited: locks}
	targetPath, targetArgs := cmd.Path, cmd.Args
	count := len(cmd.ExtraFiles)
	cmd.ExtraFiles = append(cmd.ExtraFiles, read)
	cmd.ExtraFiles = append(cmd.ExtraFiles, locks...)
	cmd.Path = exe
	cmd.Args = append([]string{exe, guardianArgument, strconv.FormatInt(int64(grace), 10), strconv.Itoa(count), strconv.Itoa(len(locks)), targetPath}, targetArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c, nil
}
func (c *control) attach(*exec.Cmd) error {
	c.read.Close()
	c.read = nil
	for _, f := range c.inherited {
		f.Close()
	}
	c.inherited = nil
	return nil
}
func (c *control) graceful(*exec.Cmd, os.Signal) { c.write.Write([]byte{'G'}) }
func (c *control) kill()                         { c.write.Write([]byte{'K'}) }
func (c *control) finish(time.Duration)          {} // Guardian exits only after its group is gone.
func (c *control) close() {
	if c.read != nil {
		c.read.Close()
	}
	if c.write != nil {
		c.write.Close()
	}
	for _, f := range c.inherited {
		f.Close()
	}
}

func init() {
	if len(os.Args) > 1 && os.Args[1] == guardianArgument {
		os.Exit(guardian(os.Args[2:]))
	}
}

func guardian(args []string) int {
	if len(args) < 5 {
		return 1
	}
	ns, e := strconv.ParseInt(args[0], 10, 64)
	if e != nil || ns <= 0 {
		return 1
	}
	count, e := strconv.Atoi(args[1])
	if e != nil || count < 0 {
		return 1
	}
	locks, e := strconv.Atoi(args[2])
	if e != nil || locks < 0 {
		return 1
	}
	// Descriptor 3+count is private parent-liveness/control IPC. Only this
	// guardian inherits the acquired lock descriptions; not Tunnel/Node children.
	parent := os.NewFile(uintptr(3+count), "launcher-liveness")
	defer parent.Close()
	unix.CloseOnExec(int(parent.Fd()))
	for i := 0; i < locks; i++ {
		fd := 3 + count + 1 + i
		unix.CloseOnExec(fd)
		defer unix.Close(fd)
	}
	if unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) != nil {
		return 1
	}
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	commands := make(chan byte, 2)
	go func() {
		var b [1]byte
		for {
			_, err := io.ReadFull(parent, b[:])
			if err != nil {
				commands <- 'G'
				return
			}
			commands <- b[0]
		}
	}()
	// A launcher dying before runtime creation must not cause a late orphan start.
	select {
	case <-commands:
		return 1
	case <-signals:
		return 1
	default:
	}
	child := exec.Command(args[3], args[5:]...)
	child.Args = append([]string{args[4]}, args[5:]...)
	child.Env = os.Environ()
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	for i := 0; i < count; i++ {
		child.ExtraFiles = append(child.ExtraFiles, os.NewFile(uintptr(3+i), "runtime-file"))
	}
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if child.Start() != nil {
		return 1
	}
	child.Env = nil
	os.Unsetenv("CONTROL_PLANE_API_KEY")
	wait := make(chan error, 1)
	go func() { wait <- child.Wait() }()
	var result error
	waited := false
	force := false
	select {
	case result = <-wait:
		force = true
		waited = true
	case b := <-commands:
		force = b == 'K'
	case <-signals:
	}
	if !force {
		syscall.Kill(-child.Process.Pid, syscall.SIGTERM)
		timer := time.NewTimer(time.Duration(ns))
		select {
		case result = <-wait:
			waited = true
		case <-timer.C:
		case <-commands:
		}
		timer.Stop()
	}
	// Only the group created for this runtime is addressed. Retain locks even
	// after the launcher is killed, until cleanup/reaping has completed.
	syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
	if !waited {
		result = <-wait
	}
	for {
		var status syscall.WaitStatus
		for {
			pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
			if pid <= 0 || err != nil {
				break
			}
		}
		if syscall.Kill(-child.Process.Pid, 0) != nil {
			break
		}
		// Fail closed: an uninterruptible owned process keeps ownership, rather
		// than releasing the lock and allowing a duplicate runtime.
		syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
		time.Sleep(10 * time.Millisecond)
	}
	if result != nil {
		if exit, ok := result.(*exec.ExitError); ok {
			if exit.ExitCode() < 0 {
				return 1
			}
			return exit.ExitCode()
		}
		return 1
	}
	return 0
}

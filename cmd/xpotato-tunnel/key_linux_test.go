//go:build linux

package main

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"reflect"
	"syscall"
	"testing"
	"time"
)

func TestTerminalKeyFixture(t *testing.T) {
	mode := os.Getenv("XPOTATO_PTY_FIXTURE")
	if mode == "" {
		return
	}
	key, e := (terminal{}).Key()
	if mode == "enter" {
		if e != nil || string(key) != "synthetic-fixture" {
			os.Exit(2)
		}
	} else if e == nil || len(key) != 0 {
		os.Exit(3)
	}
	os.Exit(42)
}
func TestRealPTYKeyCancellationRestoresTerminal(t *testing.T) {
	for _, mode := range []string{"ctrl-c", "terminate", "hangup", "enter"} {
		t.Run(mode, func(t *testing.T) {
			fd, e := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
			if e != nil {
				t.Skip("PTY unavailable")
			}
			master := os.NewFile(uintptr(fd), "private-pty")
			defer master.Close()
			if e = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); e != nil {
				t.Fatal(e)
			}
			num, e := unix.IoctlGetInt(fd, unix.TIOCGPTN)
			if e != nil {
				t.Fatal(e)
			}
			slave, e := os.OpenFile(fmt.Sprintf("/dev/pts/%d", num), os.O_RDWR, 0)
			if e != nil {
				t.Fatal(e)
			}
			defer slave.Close()
			original, e := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
			if e != nil {
				t.Fatal(e)
			}
			originalFlags, e := unix.FcntlInt(slave.Fd(), unix.F_GETFL, 0)
			if e != nil {
				t.Fatal(e)
			}
			exe, _ := os.Executable()
			cmd := exec.Command(exe, "-test.run=^TestTerminalKeyFixture$")
			cmd.Env = append(os.Environ(), "XPOTATO_PTY_FIXTURE="+mode)
			cmd.Stdin = slave
			cmd.Stdout = slave
			cmd.Stderr = slave
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer cmd.Process.Kill()
			deadline := time.Now().Add(5 * time.Second)
			for {
				hidden, e := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
				if e != nil {
					t.Fatal(e)
				}
				if hidden.Lflag&unix.ECHO == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("actual key reader did not disable echo")
				}
				time.Sleep(10 * time.Millisecond)
			}
			switch mode {
			case "ctrl-c":
				master.Write([]byte{3})
			case "terminate":
				cmd.Process.Signal(syscall.SIGTERM)
			case "hangup":
				cmd.Process.Signal(syscall.SIGHUP)
			case "enter":
				master.Write([]byte("synthetic-fixture\n"))
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case e = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("PTY reader did not cancel")
			}
			exit, ok := e.(*exec.ExitError)
			if !ok || exit.ExitCode() != 42 {
				t.Fatal("actual PTY key reader did not return the expected cancellation/result")
			}
			restored, e := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
			if e != nil || !reflect.DeepEqual(original, restored) {
				t.Fatal("exact terminal state was not restored")
			}
			restoredFlags, e := unix.FcntlInt(slave.Fd(), unix.F_GETFL, 0)
			if e != nil || originalFlags != restoredFlags {
				t.Fatal("terminal descriptor flags were not restored")
			}
			t.Log("actual PTY: masked key reader returned and restored original terminal state")
		})
	}
}

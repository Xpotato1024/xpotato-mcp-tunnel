//go:build linux

package main

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"os/signal"
	"syscall"
)

func readKey(fd int) (key []byte, err error) {
	state, e := unix.IoctlGetTermios(fd, unix.TCGETS)
	if e != nil {
		return nil, e
	}
	flags, e := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if e != nil {
		return nil, e
	}
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	hidden := *state
	hidden.Lflag &^= unix.ECHO
	hidden.Lflag |= unix.ICANON | unix.ISIG
	hidden.Iflag |= unix.ICRNL
	if e = unix.IoctlSetTermios(fd, unix.TCSETS, &hidden); e != nil {
		return nil, e
	}
	defer func() {
		if _, restore := unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags); restore != nil {
			err = errors.New("terminal descriptor restoration failed")
		}
		if restore := unix.IoctlSetTermios(fd, unix.TCSETS, state); restore != nil {
			err = errors.New("terminal restoration failed")
		}
		if err != nil {
			unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)
			for i := range key {
				key[i] = 0
			}
			key = nil
		}
	}()
	if e = unix.SetNonblock(fd, true); e != nil {
		return nil, e
	}
	// Poll synchronously: no blocked ReadPassword goroutine can turn echo off
	// again after cancellation has restored the exact saved terminal state.
	for {
		select {
		case <-signals:
			return key, errors.New("key input cancelled")
		default:
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, e := unix.Poll(fds, 100)
		if e == unix.EINTR || e == unix.EAGAIN {
			continue
		}
		if e != nil {
			return key, e
		}
		if n == 0 {
			continue
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			return key, errors.New("terminal input closed")
		}
		var b [1]byte
		n, e = unix.Read(fd, b[:])
		if e == unix.EINTR || e == unix.EAGAIN {
			continue
		}
		if e != nil || n == 0 {
			return key, errors.New("terminal input unavailable")
		}
		if b[0] == '\n' {
			return key, nil
		}
		if b[0] == '\r' {
			continue
		}
		if len(key) >= 2560 {
			return key, errors.New("key input too long")
		}
		key = append(key, b[0])
	}
}

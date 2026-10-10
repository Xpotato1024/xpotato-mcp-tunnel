//go:build !linux

package main

import "golang.org/x/term"

func readKey(fd int) ([]byte, error) { return term.ReadPassword(fd) }

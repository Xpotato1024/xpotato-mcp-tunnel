package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func operationContext(command string) (context.Context, context.CancelFunc) {
	// Text/Key prompts retain their existing signal handling: a background
	// ReadString must not swallow Ctrl+C until the user presses Enter.
	if command == "start" || command == "update" || command == "repair" || command == "doctor" || command == "rollback" {
		return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	}
	return context.Background(), func() {}
}

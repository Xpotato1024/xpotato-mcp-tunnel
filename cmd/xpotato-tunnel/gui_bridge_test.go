package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type guiHarness struct {
	t      *testing.T
	input  *io.PipeWriter
	events chan guiEvent
	done   chan struct{}
}

func newGUIHarness(t *testing.T, execute guiExecute) *guiHarness {
	t.Helper()
	in, input := io.Pipe()
	output, out := io.Pipe()
	h := &guiHarness{t: t, input: input, events: make(chan guiEvent, 32), done: make(chan struct{})}
	go func() {
		defer close(h.done)
		defer out.Close()
		_, _ = serveGUI(in, out, execute, guiEvent{Type: "hello", Supported: true})
	}()
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var event guiEvent
			if decoder.Decode(&event) != nil {
				return
			}
			h.events <- event
		}
	}()
	t.Cleanup(func() {
		h.send(guiMessage{Type: "shutdown"})
		input.Close()
		select {
		case <-h.done:
		case <-time.After(2 * time.Second):
			t.Error("GUI did not drain owned fixture")
		}
		output.Close()
		in.Close()
	})
	if h.next().Type != "hello" {
		t.Fatal("missing GUI hello")
	}
	return h
}
func (h *guiHarness) send(message guiMessage) {
	h.t.Helper()
	if err := json.NewEncoder(h.input).Encode(message); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		h.t.Fatal(err)
	}
}
func (h *guiHarness) next() guiEvent {
	h.t.Helper()
	select {
	case event := <-h.events:
		return event
	case <-time.After(2 * time.Second):
		h.t.Fatal("GUI fixture event deadline")
		return guiEvent{}
	}
}
func TestGUIConsentAndSecretNeverEchoReplies(t *testing.T) {
	secret := []byte("fixture-not-a-real-key")
	used := make(chan []byte, 1)
	h := newGUIHarness(t, func(ctx context.Context, request guiMessage, p *guiPrompt) (string, map[string]any, error) {
		yes, err := p.Consent("save fixture only")
		if err != nil || !yes {
			return "", nil, errors.New("fixture declined")
		}
		key, err := p.Key()
		used <- append([]byte(nil), key...)
		clear(key)
		return "", nil, err
	})
	h.send(guiMessage{Type: "operation", ID: "setup-1", Action: "setup"})
	if h.next().Type != "working" {
		t.Fatal("missing operation")
	}
	consent := h.next()
	if consent.Kind != "consent" {
		t.Fatal("key requested before consent")
	}
	// A stale prompt cannot consent, even when it has the right operation ID.
	h.send(guiMessage{Type: "reply", ID: "setup-1", Prompt: consent.Prompt + 99, Value: []byte("yes")})
	h.send(guiMessage{Type: "reply", ID: "setup-1", Prompt: consent.Prompt, Value: []byte("yes")})
	prompt := h.next()
	if prompt.Kind != "secret" {
		t.Fatal("missing secret input")
	}
	h.send(guiMessage{Type: "reply", ID: "setup-1", Prompt: prompt.Prompt, Value: secret})
	result := h.next()
	encoded, _ := json.Marshal(result)
	if result.Type != "complete" || strings.Contains(string(encoded), string(secret)) {
		t.Fatal("reply leaked or operation failed")
	}
	if string(<-used) != string(secret) {
		t.Fatal("fixture key did not reach the explicit prompt")
	}
}
func TestGUIMaintenanceAndOwnedConnectionAreIndependent(t *testing.T) {
	h := newGUIHarness(t, func(ctx context.Context, _ guiMessage, _ *guiPrompt) (string, map[string]any, error) {
		<-ctx.Done()
		return "", nil, ctx.Err()
	})
	h.send(guiMessage{Type: "operation", ID: "connection", Action: "start"})
	if h.next().Type != "working" {
		t.Fatal("connection not started")
	}
	h.send(guiMessage{Type: "operation", ID: "update", Action: "update"})
	if h.next().Type != "working" {
		t.Fatal("maintenance blocked by owned connection")
	}
	h.send(guiMessage{Type: "operation", ID: "second-update", Action: "repair"})
	if h.next().Type != "rejected" {
		t.Fatal("overlapping maintenance allowed")
	}
	h.send(guiMessage{Type: "cancel", ID: "unrelated-process"})
	h.send(guiMessage{Type: "cancel", ID: "update"})
	result := h.next()
	if result.ID != "update" || result.Type != "cancelled" {
		t.Fatal("cancellation escaped the selected operation", result)
	}
	h.send(guiMessage{Type: "operation", ID: "second-connection", Action: "start"})
	if h.next().Type != "rejected" {
		t.Fatal("wrong operation cancellation stopped the connection")
	}
	h.send(guiMessage{Type: "cancel", ID: "connection"})
	if result = h.next(); result.ID != "connection" || result.Type != "cancelled" {
		t.Fatal("owned connection not cancelled")
	}
}
func TestGUIRejectsArbitraryCommandsWithoutExecuting(t *testing.T) {
	called := false
	h := newGUIHarness(t, func(context.Context, guiMessage, *guiPrompt) (string, map[string]any, error) {
		called = true
		return "", nil, nil
	})
	h.send(guiMessage{Type: "operation", ID: "bad", Action: "powershell.exe"})
	if h.next().Type != "rejected" || called {
		t.Fatal("arbitrary command reached the backend")
	}
}

type guiWriteFunc func([]byte) (int, error)

func (f guiWriteFunc) Write(data []byte) (int, error) { return f(data) }

func TestGUIOutputFailureCancelsOwnedOperation(t *testing.T) {
	in, input := io.Pipe()
	defer in.Close()
	defer input.Close()
	writes := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = serveGUI(in, guiWriteFunc(func(data []byte) (int, error) {
			writes++
			if writes > 1 {
				return 0, errors.New("fixture output broken")
			}
			return len(data), nil
		}), func(ctx context.Context, _ guiMessage, _ *guiPrompt) (string, map[string]any, error) {
			<-ctx.Done()
			return "", nil, ctx.Err()
		}, guiEvent{Type: "hello", Supported: true})
	}()
	if err := json.NewEncoder(input).Encode(guiMessage{Type: "operation", ID: "owned", Action: "start"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("output failure did not cancel/drain owned fixture")
	}
}

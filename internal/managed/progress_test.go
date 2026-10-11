package managed

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStageHeartbeatsStopBeforeNextInput(t *testing.T) {
	var mu sync.Mutex
	var messages []string
	beat := make(chan struct{}, 1)
	report := func(s string) {
		mu.Lock()
		messages = append(messages, s)
		mu.Unlock()
		if strings.Contains(s, "処理中") {
			select {
			case beat <- struct{}{}:
			default:
			}
		}
	}
	err := runStage(report, "fixture install", time.Millisecond, func() error {
		select {
		case <-beat:
			return errors.New("fixture failure")
		case <-time.After(time.Second):
			t.Fatal("no heartbeat while operation was blocked")
			return nil
		}
	})
	if err == nil || !strings.Contains(err.Error(), "fixture install") {
		t.Fatal("failure lost stage", err)
	}
	mu.Lock()
	count := len(messages)
	if count < 3 || !strings.Contains(messages[count-1], "失敗") {
		t.Fatal(messages)
	}
	mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(messages) != count {
		t.Fatal("heartbeat crossed operation/input boundary")
	}
}

func TestCancelledFileWorkPreservesExistingFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "existing")
	os.WriteFile(file, []byte("unchanged fixture"), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := filesReceiptContext(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := hashFileContext(ctx, file); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	w, _ := z.Create("new-entry")
	w.Write([]byte("fixture"))
	z.Close()
	zipPath := filepath.Join(root, "fixture.zip")
	os.WriteFile(zipPath, archive.Bytes(), 0600)
	if err := extractZIPContext(ctx, zipPath, root); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "new-entry")); !os.IsNotExist(err) {
		t.Fatal("cancelled extract wrote entry")
	}
	b, _ := os.ReadFile(file)
	if string(b) != "unchanged fixture" {
		t.Fatal("cancelled operation changed existing file")
	}
}

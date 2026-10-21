package managed

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMidInventoryCancellationReportsCompletedWork(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 80; i++ {
		if e := os.WriteFile(filepath.Join(root, fmt.Sprintf("%03d", i)), []byte("fixture"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var last InventoryProgress
	e := walkFileHashesObserved(ctx, root, func(string, string) error { return nil }, func(p InventoryProgress) {
		last = p
		if p.Files == 20 {
			cancel()
		}
	})
	if !errors.Is(e, context.Canceled) || last.Files != 20 || last.Bytes != 20*7 {
		t.Fatal("cancelled walk/progress incorrect", e, last)
	}
	files, e := filesReceipt(root)
	if e != nil || len(files) != 80 {
		t.Fatal("cancel changed fixture files", e)
	}
}

func TestInventoryProgressContainsCountsWithoutPaths(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "private-fixture-name"), []byte("1234567"), 0600)
	var messages []string
	m := Manager{Progress: func(s string) { messages = append(messages, s) }}
	if e := m.inventory(context.Background(), "fixture verify", root, func(string, string) error { return nil }); e != nil {
		t.Fatal(e)
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "files 1; bytes 7 (totals unknown)") || strings.Contains(joined, "private-fixture-name") || strings.Contains(joined, root) {
		t.Fatal("unhelpful or unsafe progress", joined)
	}
}

func TestDownloadCancellationPropagatesWithoutContinuingInstall(t *testing.T) {
	requested := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(requested); <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- downloadContext(ctx, server.URL, filepath.Join(t.TempDir(), "download"), strings.Repeat("0", 64))
	}()
	select {
	case <-requested:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("fixture request not received")
	}
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal("cancel not preserved", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled download blocked")
	}
}

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

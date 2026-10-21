package managed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkUpdateInventoryPasses(b *testing.B) {
	root := b.TempDir()
	m := fixtureManifest(b)
	syntheticRuntime(b, root, m)
	var payloadBytes int64
	// Match the historical receipt's file count, never its private content/path.
	for i := 0; i < 26061; i++ {
		data := []byte(strings.Repeat("x", 64))
		if i%257 == 0 {
			data = []byte(strings.Repeat("y", 256*1024))
		}
		p := filepath.Join(root, fmt.Sprintf("synthetic-%05d", i))
		if e := os.WriteFile(p, data, 0600); e != nil {
			b.Fatal(e)
		}
		payloadBytes += int64(len(data))
	}
	files, e := filesReceipt(root)
	if e != nil {
		b.Fatal(e)
	}
	receipt, _ := json.Marshal(Receipt{Manifest: m, Files: files})
	os.WriteFile(filepath.Join(root, "receipt.json"), receipt, 0600)
	for _, tc := range []struct {
		name   string
		passes int
		record bool
	}{{"new-before-3-walks", 3, true}, {"new-after-2-walks", 2, true}, {"reuse-before-2-walks", 2, false}, {"reuse-after-1-walk", 1, false}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(payloadBytes * int64(tc.passes))
			b.ReportMetric(float64(tc.passes), "walks/op")
			b.ReportMetric(float64(len(files)*tc.passes), "files/op")
			for b.Loop() {
				for pass := 0; pass < tc.passes; pass++ {
					if pass == 0 && tc.record {
						if _, e := filesReceipt(root); e != nil {
							b.Fatal(e)
						}
					} else {
						if e := verifyDirectory(root, m); e != nil {
							b.Fatal(e)
						}
					}
				}
			}
		})
	}
}

func TestPinnedFilesCannotBeChangedAlongWithReceipt(t *testing.T) {
	for _, path := range []string{executable(), "package-lock.json", "stdio.mjs", "activity.mjs", "licensing.mjs", "node_modules/@wonderwhy-er/desktop-commander/dist/index.js"} {
		t.Run(path, func(t *testing.T) {
			m := fixtureManifest(t)
			root := t.TempDir()
			syntheticRuntime(t, root, m)
			os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte("changed with receipt"), 0600)
			files, e := filesReceipt(root)
			if e != nil {
				t.Fatal(e)
			}
			b, _ := json.Marshal(Receipt{Manifest: m, Files: files})
			os.WriteFile(filepath.Join(root, "receipt.json"), b, 0600)
			if e = verifyDirectory(root, m); e == nil {
				t.Fatal("compiled-in pin/marker bypassed by receipt edit")
			}
		})
	}
}

func syntheticRuntime(t testing.TB, dir string, m Manifest) {
	t.Helper()
	files := map[string][]byte{executable(): []byte("synthetic runtime"), "package-lock.json": mustAsset(t, "package-lock.json"), "stdio.mjs": mustAsset(t, "stdio.mjs"), "activity.mjs": mustAsset(t, "activity.mjs"), "licensing.mjs": mustAsset(t, "licensing.mjs"), "node_modules/@wonderwhy-er/desktop-commander/dist/index.js": []byte(marker + "\n"), "node/synthetic-node": []byte("fixture node")}
	node, _ := nodeFiles(dir)
	relative, _ := filepath.Rel(dir, node)
	files[relative] = []byte("fixture node executable")
	for p, b := range files {
		dest := filepath.Join(dir, filepath.FromSlash(p))
		if e := os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(dest, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	h, e := filesReceipt(dir)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(Receipt{Manifest: m, Files: h})
	if e = os.WriteFile(filepath.Join(dir, "receipt.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
}
func mustAsset(t testing.TB, name string) []byte {
	t.Helper()
	b, e := assets.ReadFile("assets/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func fixtureManifest(t testing.TB) Manifest {
	t.Helper()
	m, e := manifest()
	if e != nil {
		t.Fatal(e)
	}
	m.ExecutableSHA = hashBytes([]byte("synthetic runtime"))
	return m
}

func TestRuntimeRecipeIsIndependentOfCoreAndIncludesContent(t *testing.T) {
	m := fixtureManifest(t)
	id := releaseID(m)
	if !strings.HasPrefix(id, "runtime-v1-") || strings.Contains(id, "core-") {
		t.Fatal("recipe tied to core", id)
	}
	m.NodeSHA = strings.Repeat("f", 64)
	if id == releaseID(m) {
		t.Fatal("dependency bytes omitted from recipe")
	}
}

func TestUpdateRuntimeReuseUsesOnlyBoundedReadiness(t *testing.T) {
	for _, change := range []string{"none", "change", "missing", "extra", "receipt"} {
		t.Run(change, func(t *testing.T) {
			home := t.TempDir()
			m := fixtureManifest(t)
			id := "core-0.2.6-" + m.Platform + "-tunnel-" + tunnelVersion + "-dc-" + commanderVersion + "-node-" + nodeVersion
			dir := filepath.Join(home, "releases", id)
			syntheticRuntime(t, dir, m)
			switch change {
			case "change":
				node, _ := nodeFiles(dir)
				os.WriteFile(node, []byte("changed"), 0600)
			case "missing":
				node, _ := nodeFiles(dir)
				os.Remove(node)
			case "extra":
				os.WriteFile(filepath.Join(dir, "extra"), nil, 0600)
			case "receipt":
				os.WriteFile(filepath.Join(dir, "receipt.json"), []byte("{}"), 0600)
			}
			walks, populates := 0, 0
			manager := Manager{runtimeManifest: func() (Manifest, error) { return m, nil }, populate: func(context.Context, string, Manifest) error { populates++; return errors.New("must reuse") }, inventoryStarted: func() { walks++ }}
			p, e := manager.PrepareRuntime(home, id)
			if e != nil {
				t.Fatal(e)
			}
			defer p.Close()
			if walks != 0 || populates != 0 {
				t.Fatal("prepared by hashing/downloading")
			}
			got, e := p.Verify()
			if (change == "none" || change == "extra") != (e == nil) {
				t.Fatal("incorrect integrity result", e)
			}
			if e == nil && got != id {
				t.Fatal("legacy generation copied/rekeyed")
			}
			if walks != 0 || populates != 0 {
				t.Fatal("duplicate scan/population", walks, populates)
			}
		})
	}
}

func TestNewRuntimePreparedWithoutExpandedTreeInventory(t *testing.T) {
	home := t.TempDir()
	m := fixtureManifest(t)
	walks := 0
	manager := Manager{runtimeManifest: func() (Manifest, error) { return m, nil }, populate: func(_ context.Context, dir string, m Manifest) error { syntheticRuntime(t, dir, m); return nil }, inventoryStarted: func() { walks++ }}
	p, e := manager.PrepareRuntime(home, "")
	if e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(home, "releases", releaseID(m))
	if _, e = os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("unverified final runtime published")
	}
	id, e := p.Verify()
	p.Close()
	if e != nil || id != releaseID(m) || walks != 0 {
		t.Fatal(id, e, walks)
	}
	if e = manager.Verify(home, id); e != nil || walks != 0 {
		t.Fatal("normal Start performed full scan or failed readiness", e, walks)
	}
}

func TestCancelledRuntimePreparationRemovesOnlyItsStage(t *testing.T) {
	home := t.TempDir()
	m := fixtureManifest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	keep := filepath.Join(home, "releases", ".stage-other-operation")
	os.MkdirAll(keep, 0700)
	manager := Manager{Context: ctx, runtimeManifest: func() (Manifest, error) { return m, nil }, populate: func(_ context.Context, dir string, m Manifest) error { syntheticRuntime(t, dir, m); return nil }}
	p, e := manager.PrepareRuntime(home, "")
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	if _, e = p.Verify(); !errors.Is(e, context.Canceled) {
		t.Fatal("cancel lost", e)
	}
	p.Close()
	entries, _ := os.ReadDir(filepath.Join(home, "releases"))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".stage-") && entry.Name() != ".stage-other-operation" {
			t.Fatal("own stage leaked")
		}
	}
	if _, e = os.Stat(keep); e != nil {
		t.Fatal("unrelated stage removed")
	}
}

func TestCancelledRunDoesNotPrepareOrLaunchRuntime(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := (Manager{Context: ctx}).Run(home, core.Device{}, []byte("synthetic")); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled Run continued", e)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("cancelled Run created state")
	}
}

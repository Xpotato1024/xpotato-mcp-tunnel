package managed

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLaunchReceiptAndNormalReuseNeverInventoryExpandedTree(t *testing.T) {
	home := t.TempDir()
	manifest := fixtureManifest(t)
	id := releaseID(manifest)
	root := filepath.Join(home, "releases", id)
	syntheticRuntime(t, root, manifest)
	receipt, err := captureLaunchReceipt(context.Background(), root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 2 || len(receipt.Files) != 7 || receipt.Scope != launchReceiptScope {
		t.Fatal(receipt)
	}
	data, _ := json.Marshal(receipt)
	os.WriteFile(filepath.Join(root, "receipt.json"), data, 0600)
	// Unrecorded transitive content is outside the bounded check by design.
	// This also catches accidental reuse of the legacy exhaustive comparator.
	os.WriteFile(filepath.Join(root, "node", "synthetic-node"), []byte("transitive alteration"), 0600)
	walks := 0
	manager := Manager{runtimeManifest: func() (Manifest, error) { return manifest, nil }, inventoryStarted: func() { walks++ }}
	if err = manager.Verify(home, id); err != nil {
		t.Fatal(err)
	}
	if walks != 0 {
		t.Fatal("normal check scanned expanded tree", walks)
	}
	manager.populate = func(_ context.Context, dir string, m Manifest) error { syntheticRuntime(t, dir, m); return nil }
	if err = manager.DeepVerify(home, id); err == nil {
		t.Fatal("explicit deep diagnosis missed transitive alteration")
	}
	if walks != 2 {
		t.Fatal("deep diagnosis did not compare complete installed tree", walks)
	}
}

func TestReadinessRejectsCriticalDeletionAndTraversal(t *testing.T) {
	manifest := fixtureManifest(t)
	for _, change := range []string{"missing", "hash", "path", "scope"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			syntheticRuntime(t, root, manifest)
			receipt, err := captureLaunchReceipt(context.Background(), root, manifest)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "missing":
				node, _ := nodeFiles(root)
				os.Remove(node)
			case "hash":
				receipt.Files[executable()] = hashBytes([]byte("wrong"))
			case "path":
				receipt.Files["../escape"] = hashBytes(nil)
			case "scope":
				receipt.Scope = "entire-runtime"
			}
			data, _ := json.Marshal(receipt)
			os.WriteFile(filepath.Join(root, "receipt.json"), data, 0600)
			if err = (Manager{}).verifyReadiness(context.Background(), root, manifest); err == nil {
				t.Fatal("invalid readiness accepted")
			}
		})
	}
}

func TestDeepRejectsReceiptMismatchBeforeReferenceAcquisition(t *testing.T) {
	home := t.TempDir()
	manifest := fixtureManifest(t)
	id := releaseID(manifest)
	root := filepath.Join(home, "releases", id)
	syntheticRuntime(t, root, manifest)
	receipt, err := captureLaunchReceipt(context.Background(), root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Files[executable()] = hashBytes([]byte("changed receipt only"))
	data, _ := json.Marshal(receipt)
	os.WriteFile(filepath.Join(root, "receipt.json"), data, 0600)
	acquisitions := 0
	manager := Manager{runtimeManifest: func() (Manifest, error) { return manifest, nil }, populate: func(context.Context, string, Manifest) error { acquisitions++; return nil }}
	if err = manager.DeepVerify(home, id); err == nil {
		t.Fatal("deep diagnosis ignored receipt mismatch")
	}
	if acquisitions != 0 {
		t.Fatal("unnecessary reference acquisition", acquisitions)
	}
}

func TestRepairPublishesNewGenerationAndPreservesOldTree(t *testing.T) {
	home := t.TempDir()
	manifest := fixtureManifest(t)
	old := releaseID(manifest)
	root := filepath.Join(home, "releases", old)
	syntheticRuntime(t, root, manifest)
	node, _ := nodeFiles(root)
	os.WriteFile(node, []byte("damaged old node"), 0600)
	manager := Manager{ForceRepair: true, runtimeManifest: func() (Manifest, error) { return manifest, nil }, populate: func(_ context.Context, dir string, m Manifest) error { syntheticRuntime(t, dir, m); return nil }}
	prepared, err := manager.PrepareRuntime(home, old)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	id, err := prepared.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if id == old || !isRepairRuntimeID(manifest, id) {
		t.Fatal("repair reused old generation", id)
	}
	before, _ := os.ReadFile(node)
	if string(before) != "damaged old node" {
		t.Fatal("repair modified old generation")
	}
	if err = manager.Verify(home, id); err != nil {
		t.Fatal(err)
	}
}

func TestRepairFailureKeepsOldGenerationAndCleansOnlyOwnStage(t *testing.T) {
	home := t.TempDir()
	manifest := fixtureManifest(t)
	old := releaseID(manifest)
	root := filepath.Join(home, "releases", old)
	syntheticRuntime(t, root, manifest)
	manager := Manager{ForceRepair: true, runtimeManifest: func() (Manifest, error) { return manifest, nil }, populate: func(_ context.Context, dir string, m Manifest) error {
		syntheticRuntime(t, dir, m)
		return errors.New("fixture interrupted")
	}}
	if _, err := manager.PrepareRuntime(home, old); err == nil {
		t.Fatal("repair failure ignored")
	}
	if err := (Manager{}).verifyReadiness(context.Background(), root, manifest); err != nil {
		t.Fatal("existing runtime changed", err)
	}
	entries, _ := os.ReadDir(filepath.Join(home, "releases"))
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != old {
			t.Fatal("failed repair leaked a generation", entry.Name())
		}
	}
}

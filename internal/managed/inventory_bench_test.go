package managed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// Reproduce the adopted 0.2.4 per-file hash allocation for the same fixture.
func legacyInventory(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	return files, err
}

func BenchmarkRuntimeInventory(b *testing.B) {
	root := b.TempDir()
	for i := 0; i < 256; i++ {
		if err := os.WriteFile(filepath.Join(root, strconv.Itoa(i)+".txt"), []byte("fixed synthetic file data\n"), 0600); err != nil {
			b.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		scan func(string) (map[string]string, error)
	}{{"before-0.2.4", legacyInventory}, {"after-reused-buffer", filesReceipt}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for b.Loop() {
				files, err := tc.scan(root)
				if err != nil || len(files) != 256 {
					b.Fatal(err)
				}
			}
			runtime.ReadMemStats(&after)
			b.ReportMetric(float64(after.NumGC-before.NumGC)/float64(b.N), "gc/op")
		})
	}
	expected, _ := filesReceipt(root)
	b.Run("after-stream-verify", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			count := 0
			err := walkFileHashes(context.Background(), root, func(p, h string) error {
				if expected[p] != h {
					b.Fatal("hash changed")
				}
				count++
				return nil
			})
			if err != nil || count != 256 {
				b.Fatal(err)
			}
		}
	})
}

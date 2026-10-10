//go:build linux

package managed

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestNodeArchiveRootDirectoryAndSkippedAliases(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "node.tar.gz")
	file, e := os.Create(archive)
	if e != nil {
		t.Fatal(e)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	prefix := "node-v" + nodeVersion + "-linux-x64/"
	tw.WriteHeader(&tar.Header{Name: prefix, Typeflag: tar.TypeDir, Mode: 0755})
	tw.WriteHeader(&tar.Header{Name: prefix + "bin/node", Typeflag: tar.TypeReg, Mode: 0755, Size: 7})
	tw.Write([]byte("fixture"))
	tw.WriteHeader(&tar.Header{Name: prefix + "bin/npm", Typeflag: tar.TypeSymlink, Linkname: "../lib/node_modules/npm/bin/npm-cli.js", Mode: 0755})
	tw.Close()
	gz.Close()
	file.Close()
	if e = extractNode(archive, root); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(root, "node", "bin", "node"))
	if e != nil || string(b) != "fixture" {
		t.Fatal("Node root not extracted")
	}
	if _, e = os.Lstat(filepath.Join(root, "node", "bin", "npm")); !os.IsNotExist(e) {
		t.Fatal("unused alias extracted")
	}
}

package managed

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformManifestsKeepNativeArchitectureAndPins(t *testing.T) {
	for _, pair := range [][2]string{{"windows", "amd64"}, {"linux", "amd64"}, {"darwin", "amd64"}, {"darwin", "arm64"}} {
		m, err := manifestFor(pair[0], pair[1])
		if err != nil {
			t.Fatal(pair, err)
		}
		if m.Platform != pair[0]+"-"+pair[1] || !strings.HasSuffix(m.TunnelURL, m.Platform+".zip") {
			t.Fatal("architecture mismatch", m)
		}
		for _, sum := range []string{m.NodeSHA, m.TunnelSHA, m.ExecutableSHA} {
			b, err := hex.DecodeString(sum)
			if err != nil || len(b) != 32 {
				t.Fatal("invalid platform pin", pair)
			}
		}
	}
	for _, pair := range [][2]string{{"windows", "arm64"}, {"linux", "arm64"}, {"freebsd", "amd64"}} {
		if _, err := manifestFor(pair[0], pair[1]); err == nil {
			t.Fatal("unreviewed platform enabled", pair)
		}
	}
}

func TestNodeExtractionAcceptsOnlyTheSelectedMacArchitecture(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		m, _ := manifestFor("darwin", arch)
		prefix := strings.TrimSuffix(m.NodeURL[strings.LastIndex(m.NodeURL, "/")+1:], ".tar.gz") + "/"
		for _, wrong := range []bool{false, true} {
			root := t.TempDir()
			archive := filepath.Join(root, "node.tar.gz")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(f)
			tw := tar.NewWriter(gz)
			name := prefix + "bin/node"
			if wrong {
				name = "node-v" + nodeVersion + "-linux-x64/bin/node"
			}
			if err = tw.WriteHeader(&tar.Header{Name: name, Mode: 0700, Size: 7, Typeflag: tar.TypeReg}); err != nil {
				t.Fatal(err)
			}
			tw.Write([]byte("fixture"))
			tw.Close()
			gz.Close()
			f.Close()
			err = extractNodePlatform(context.Background(), archive, root, "darwin", arch)
			if (err != nil) != wrong {
				t.Fatal(arch, wrong, err)
			}
			if !wrong {
				b, err := os.ReadFile(filepath.Join(root, "node", "bin", "node"))
				if err != nil || string(b) != "fixture" {
					t.Fatal(err)
				}
			}
		}
	}
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerPayloadCheckRejectsTamperAndTraversal(t *testing.T) {
	for _, change := range []string{"none", "tamper", "missing", "traversal"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{"xpotato-tunnel.exe": "fixture-not-executed", "build-info.json": fmt.Sprintf(`{"version":%q,"platform":"windows-amd64","dirty":false}`, core.Version), "README.md": "readme", "licenses/LICENSE": "license", "licenses/COPYRIGHT": "copyright", "licenses/THIRD-PARTY-NOTICES.txt": "notices"}
			var manifest strings.Builder
			for name, data := range files {
				dest := filepath.Join(root, filepath.FromSlash(name))
				os.MkdirAll(filepath.Dir(dest), 0700)
				os.WriteFile(dest, []byte(data), 0600)
				hash := sha256.Sum256([]byte(data))
				fmt.Fprintf(&manifest, "%s  %s\n", hex.EncodeToString(hash[:]), name)
			}
			switch change {
			case "tamper":
				os.WriteFile(filepath.Join(root, "xpotato-tunnel.exe"), []byte("changed"), 0600)
			case "missing":
				os.Remove(filepath.Join(root, "licenses", "LICENSE"))
			case "traversal":
				fmt.Fprintf(&manifest, "%s  ../foreign\n", strings.Repeat("0", 64))
			}
			os.WriteFile(filepath.Join(root, "SHA256SUMS"), []byte(manifest.String()), 0600)
			err := checkPackage(root)
			if (err == nil) != (change == "none") {
				t.Fatal(change, err)
			}
		})
	}
}

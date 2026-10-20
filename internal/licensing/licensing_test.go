package licensing

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCanonicalNotices(t *testing.T) {
	for name, embedded := range map[string]string{"LICENSE": Text, "COPYRIGHT": Attribution} {
		b, err := os.ReadFile("../../" + name)
		if err != nil || string(b) != embedded {
			t.Fatalf("canonical %s differs: %v", name, err)
		}
	}
	if !strings.Contains(Text, "GNU AFFERO GENERAL PUBLIC LICENSE") || !strings.Contains(Attribution, "Xpotato.net / Xpotato1024") {
		t.Fatal("missing legal text or credit")
	}
}

func TestSourceArchiveBuild(t *testing.T) {
	previous := BuildCommit
	t.Cleanup(func() { BuildCommit = previous })
	BuildCommit = strings.Repeat("a", 40)
	info := Info("0.2.10")
	if info.Modified || !strings.HasSuffix(info.Source, BuildCommit) || !strings.HasSuffix(info.SourceBundle, "0.2.10-source.tar.gz") {
		t.Fatalf("bad offer: %+v", info)
	}
	var decoded Information
	if err := json.Unmarshal([]byte(JSON("0.2.10")), &decoded); err != nil || decoded.License != "AGPL-3.0-only" {
		t.Fatal("invalid source JSON")
	}
	BuildCommit = "unversioned"
	info = Info("0.2.10")
	if !info.Modified || info.Source != "" || info.SourceBundle != "" {
		t.Fatal("unversioned build claims exact public source")
	}
}

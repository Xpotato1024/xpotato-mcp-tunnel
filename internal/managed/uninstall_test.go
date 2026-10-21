package managed

import (
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallPreservesModifiedWrapperAndAllUserData(t *testing.T) {
	home := t.TempDir()
	identity, err := platform.CurrentIdentity()
	if err != nil {
		t.Fatal(err)
	}
	files := shimFiles(home, "unused", filepath.Join(home, "bin"))
	record := commandRecord{Owner: identity.Owner, Files: map[string]string{}}
	var changed string
	for name, data := range files {
		os.MkdirAll(filepath.Dir(name), 0700)
		os.WriteFile(name, data, 0600)
		record.Files[name] = hashBytes(data)
		changed = name
	}
	saveCommandRecord(filepath.Join(home, "command-install.json"), record)
	os.WriteFile(changed, []byte("user changed this wrapper"), 0600)
	keep := filepath.Join(home, "profile-fixture.json")
	os.WriteFile(keep, []byte("retained nonsecret fixture"), 0600)
	if err = UninstallEntrypoints(home); err == nil {
		t.Fatal("modified wrapper removed")
	}
	for name := range files {
		if _, err = os.Stat(name); err != nil {
			t.Fatal("preflight removed a wrapper", err)
		}
	}
	if _, err = os.Stat(keep); err != nil {
		t.Fatal("user data changed", err)
	}
}

func TestUninstallRejectsUnexpectedPathBeforeRemovingAnything(t *testing.T) {
	home := t.TempDir()
	identity, err := platform.CurrentIdentity()
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(home, "foreign-file")
	os.WriteFile(foreign, []byte("not a wrapper"), 0600)
	saveCommandRecord(filepath.Join(home, "command-install.json"), commandRecord{Owner: identity.Owner, Files: map[string]string{foreign: hashBytes([]byte("not a wrapper"))}})
	if err = UninstallEntrypoints(home); err == nil {
		t.Fatal("unexpected path accepted")
	}
	if _, err = os.Stat(foreign); err != nil {
		t.Fatal("foreign file removed")
	}
}

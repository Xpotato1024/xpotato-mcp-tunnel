package managed

import (
	"errors"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/lockfile"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"os"
	"path/filepath"
)

// Remove only this owner's unchanged wrappers. Profiles, generation trees,
// credential references and Vault contents are deliberately retained.
func UninstallEntrypoints(home string) error {
	if err := validateCommandPath(home); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(home, "command-install.json")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	identity, err := platform.CurrentIdentity()
	if err != nil {
		return errors.New("uninstall owner unavailable")
	}
	unlock, err := lockfile.Acquire(filepath.Join(home, "command-install.lock"), identity.Owner)
	if err != nil {
		return err
	}
	defer unlock()
	record, err := readCommandRecord(home)
	if err != nil {
		return err
	}
	if record.Owner == "" {
		return nil
	}
	allowed := shimFiles(home, "unused", filepath.Join(home, "bin"))
	if record.Directory != "" {
		if err = validateCommandPath(record.Directory); err != nil {
			return err
		}
		for name, data := range shimFiles(home, "unused", record.Directory) {
			allowed[name] = data
		}
	}
	// Preflight the complete small wrapper set before removing any file.
	for name, sum := range record.Files {
		if _, ok := allowed[name]; !ok {
			return errors.New("uninstall record contains an unexpected path; preserved")
		}
		st, e := os.Lstat(name)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("command entrypoint is not a regular file; preserved")
		}
		got, e := hashFile(name)
		if e != nil || got != sum {
			return errors.New("command entrypoint changed; uninstall stopped and file preserved")
		}
	}
	for name := range record.Files {
		if err = os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return saveCommandRecord(filepath.Join(home, "command-install.json"), commandRecord{Owner: identity.Owner, Files: map[string]string{}})
}

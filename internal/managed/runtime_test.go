package managed

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeArgumentPreservesQuotedSpaces(t *testing.T) {
	if os.Getenv("XPOTATO_TEST_HELPER") == "1" {
		if os.Args[len(os.Args)-1] != "'C:/Program Files/nodejs/node.exe' 'C:/My App/stdio.mjs' --no-onboarding" {
			os.Exit(2)
		}
		os.Exit(42)
	}
	command, e := Command("C:/Program Files/nodejs/node.exe", "C:/My App/stdio.mjs")
	if e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, "-test.run=TestNativeArgumentPreservesQuotedSpaces", "--", command)
	cmd.Env = append(os.Environ(), "XPOTATO_TEST_HELPER=1")
	e = cmd.Run()
	if exit, ok := e.(*exec.ExitError); !ok || exit.ExitCode() != 42 {
		t.Fatalf("native command argument changed: %v", e)
	}
}
func TestQuoteRejectsUnsupportedPaths(t *testing.T) {
	for _, p := range []string{"bad'path", "bad\npath"} {
		if _, e := Command(p, "stdio"); e == nil {
			t.Fatal("unsafe path")
		}
	}
}
func TestArchiveTraversalAndLinksRejected(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:/absolute", "bad\\path"} {
		if _, e := safeArchivePath(t.TempDir(), name); e == nil {
			t.Fatal(name)
		}
	}
	root := t.TempDir()
	archive := filepath.Join(root, "fixture.zip")
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "link"}
	h.SetMode(os.ModeSymlink | 0700)
	w, _ := z.CreateHeader(h)
	w.Write([]byte("../escape"))
	z.Close()
	os.WriteFile(archive, b.Bytes(), 0600)
	if e := extractZIP(archive, filepath.Join(root, "out")); e == nil {
		t.Fatal("symlink accepted")
	}
}
func TestChromeSuppressionRequiresExactPinnedCall(t *testing.T) {
	b := []byte("    // Preemptively check/download Chrome for PDF generation (runs in background)\n    ensureChromeAvailable();")
	patched, e := patchCommander(b)
	if e != nil || !strings.Contains(string(patched), marker) || chromeCall.Match(patched) {
		t.Fatal(e)
	}
	for _, input := range [][]byte{[]byte("unknown"), append(append(append([]byte{}, b...), '\n'), b...)} {
		if _, e = patchCommander(input); e == nil {
			t.Fatal("ambiguous/unknown patch accepted")
		}
	}
}
func TestInstallNPMEnvironmentDoesNotInheritConfigOrKeys(t *testing.T) {
	e := cleanNPMEnv([]string{"PATH=fixture", "npm_config_userconfig=private", "NPM_CONFIG_REGISTRY=private", "NPM_TOKEN=synthetic", "CONTROL_PLANE_API_KEY=synthetic", "NODE_OPTIONS=--require=private"})
	if len(e) != 1 || e[0] != "PATH=fixture" {
		t.Fatal("private npm configuration/key inherited")
	}
}
func TestShimsAreThinLocalArgumentForwarders(t *testing.T) {
	home := filepath.Join(t.TempDir(), "Home With Spaces")
	if e := InstallCommand(home); e != nil {
		t.Fatal(e)
	}
	entries, e := os.ReadDir(filepath.Join(home, "bin"))
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		b, _ := os.ReadFile(filepath.Join(home, "bin", entry.Name()))
		if strings.Contains(string(b), "npm") || strings.Contains(string(b), "git ") || strings.Contains(string(b), "key") {
			t.Fatal("business logic in shim")
		}
		if !strings.Contains(string(b), "--home") {
			t.Fatal("home argument missing")
		}
	}
}

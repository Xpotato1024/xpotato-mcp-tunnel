package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDoctorDoesNotRequireDefaultDevice(t *testing.T) {
	command, _, e := parse([]string{"doctor", "--home", t.TempDir()})
	if e != nil || command != "doctor" {
		t.Fatal("diagnostics requires completed setup")
	}
}

func TestDeepDiagnosisIsExplicitAndDeviceScoped(t *testing.T) {
	home := t.TempDir()
	for _, command := range []string{"setup", "update", "repair", "maintenance", "start"} {
		if _, _, err := parse([]string{command, "--home", home, "--device", "fixture", "--deep"}); err == nil {
			t.Fatal("deep diagnosis accepted by normal path", command)
		}
	}
	command, o, err := parse([]string{"doctor", "--home", home, "--device", "fixture", "--deep"})
	if err != nil || command != "doctor" || !o.Deep {
		t.Fatal(command, o, err)
	}
}

func TestKeyCannotBePassedAsCLIOption(t *testing.T) {
	if _, _, e := parse([]string{"setup", "--api-key", "synthetic"}); e == nil {
		t.Fatal("key accepted as argument")
	}
}

func TestActivityDetailsRequireExplicitStart(t *testing.T) {
	for _, command := range []string{"setup", "doctor", "update", "credentials", "plugin"} {
		if _, _, err := parse([]string{command, "--activity-details"}); err == nil {
			t.Fatal("details accepted outside start", command)
		}
	}
	_, options, err := parse([]string{"setup", "--home", t.TempDir()})
	if err != nil || options.ActivityDetails {
		t.Fatal("details enabled by default", err)
	}
}

func TestDiagnosticsAreExplicitAndStartOnly(t *testing.T) {
	for _, command := range []string{"setup", "doctor", "update", "rollback", "credentials", "plugin"} {
		if _, _, err := parse([]string{command, "--diagnostics"}); err == nil {
			t.Fatal("diagnostics accepted outside start", command)
		}
	}
	for _, enabled := range []bool{false, true} {
		args := []string{"start", "--home", t.TempDir(), "--device", "fixture"}
		if enabled {
			args = append(args, "--diagnostics")
		}
		_, options, err := parse(args)
		if err != nil || options.Diagnostics != enabled || options.ActivityDetails {
			t.Fatal("diagnostics changed argument disclosure", err, options)
		}
	}
}
func TestHelpAndVersionNeedNoIdentityOrHome(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"version"}, {"about"}, {"license"}, {"source"}} {
		if _, _, e := parse(args); e != nil {
			t.Fatal(e)
		}
	}
}

func TestExplicitEnvFileLoadsNonsecretDataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Allowed With Spaces")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(t.TempDir(), "setup.env")
	if e := os.WriteFile(file, []byte("DEVICE=work-pc\nTUNNEL_ID=tunnel_example\nALLOW_DIR_1='"+root+"'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("DEVICE", "ambient-device")
	t.Setenv("TUNNEL_ID", "tunnel_ambient")
	t.Setenv("ALLOW_DIR_1", "ambient-root")
	home := filepath.Join(t.TempDir(), "isolated home")
	command, o, e := parse([]string{"setup", "--env-file", file, "--home", home})
	if e != nil || command != "setup" || o.EnvFile != file || o.Home != home || o.Device != "work-pc" || o.Tunnel != "tunnel_example" || len(o.Allow) != 1 || o.Allow[0] != root {
		t.Fatal("explicit file data not preserved", e)
	}
	_, interactive, e := parse([]string{"setup", "--home", home})
	if e != nil || interactive.Device != "" || interactive.Tunnel != "" || len(interactive.Allow) != 0 || interactive.EnvFile != "" {
		t.Fatal("ambient settings were adopted", e)
	}
	if _, e := os.Stat(home); !os.IsNotExist(e) {
		t.Fatal("parse wrote setup state")
	}
}

func TestEnvFileConflictsRejectBeforeReading(t *testing.T) {
	for _, command := range []string{"setup", "start", "doctor"} {
		if _, _, e := parse([]string{command, "--env-file", ""}); e == nil {
			t.Fatal("empty explicit env path accepted", command)
		}
	}
	for _, command := range []string{"start", "doctor", "update", "rollback", "credentials", "plugin"} {
		if _, _, e := parse([]string{command, "--env-file", "does-not-exist.env"}); e == nil {
			t.Fatal("non-setup accepted env file", command)
		}
	}
	for _, option := range []string{"--device", "--tunnel-id", "--allow"} {
		for _, value := range []string{"fixture", ""} {
			if _, _, e := parse([]string{"setup", "--env-file", "does-not-exist.env", option, value}); e == nil {
				t.Fatal("conflicting option accepted", option)
			}
		}
	}
}

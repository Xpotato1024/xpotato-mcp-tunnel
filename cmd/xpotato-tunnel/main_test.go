package main

import "testing"

func TestDoctorDoesNotRequireDefaultDevice(t *testing.T) {
	command, _, e := parse([]string{"doctor", "--home", t.TempDir()})
	if e != nil || command != "doctor" {
		t.Fatal("diagnostics requires completed setup")
	}
}

func TestKeyCannotBePassedAsCLIOption(t *testing.T) {
	if _, _, e := parse([]string{"setup", "--api-key", "synthetic"}); e == nil {
		t.Fatal("key accepted as argument")
	}
}
func TestHelpAndVersionNeedNoIdentityOrHome(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"version"}} {
		if _, _, e := parse(args); e != nil {
			t.Fatal(e)
		}
	}
}

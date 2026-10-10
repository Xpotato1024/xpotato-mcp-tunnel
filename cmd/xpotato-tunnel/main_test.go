package main

import "testing"

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

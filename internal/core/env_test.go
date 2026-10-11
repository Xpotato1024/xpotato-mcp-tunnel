package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnvLiteralWindowsPaths(t *testing.T) {
	s := "# nonsecret\r\nDEVICE='work-pc'\r\nTUNNEL_ID=tunnel_example\r\nALLOW_DIR_1='D:\\Apps With Spaces'\r\nALLOW_DIR_2='E:\\WSL'\r\n"
	o, e := parseSetupEnv([]byte(s))
	if e != nil || o.Device != "work-pc" || o.Tunnel != "tunnel_example" ||
		!reflect.DeepEqual(o.Allow, []string{"D:\\Apps With Spaces", "E:\\WSL"}) {
		t.Fatal("literal Windows paths were changed", o, e)
	}
}

func TestEnvRejectsUnknownDuplicateInvalidAndSecretsWithoutValues(t *testing.T) {
	valid := "DEVICE=work-pc\nTUNNEL_ID=tunnel_example\nALLOW_DIR_1=/fixture\n"
	for _, input := range []string{
		valid + "API_KEY=synthetic-sensitive-value\n", valid + "UNKNOWN=synthetic-sensitive-value\n",
		valid + "DEVICE=other\n", strings.ReplaceAll(valid, "ALLOW_DIR_1", "ALLOW_DIR_2"),
		valid + "ALLOW_DIR_01=/other\n", valid + "ALLOW_DIR_101=/other\n",
		"DEVICE=$(synthetic-sensitive-value)\n", valid + "export DEVICE=other\n",
		"DEVICE=\"work-pc\"\nTUNNEL_ID=tunnel_example\nALLOW_DIR_1=/fixture\n",
		valid + "ALLOW_DIR_2='unclosed\n", valid + "ALLOW_DIR_2=/bad\x00path\n",
	} {
		_, e := parseSetupEnv([]byte(input))
		if e == nil || strings.Contains(e.Error(), "synthetic-sensitive-value") {
			t.Fatal("accepted invalid data or exposed value", e)
		}
	}
}

func TestEnvReadBOMUTF8LimitsAndExistingDirectories(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "setup.env")
	valid := "DEVICE=work-pc\nTUNNEL_ID=tunnel_example\nALLOW_DIR_1='" + root + "'\n"
	os.WriteFile(file, append([]byte{0xef, 0xbb, 0xbf}, []byte(valid)...), 0600)
	o, e := ReadSetupEnv(file)
	if e != nil || o.EnvFile != file || o.Allow[0] != root {
		t.Fatal(o, e)
	}
	for _, b := range [][]byte{[]byte(valid + "ALLOW_DIR_2='" + root + "'\n"), []byte(strings.Replace(valid, root, "relative", 1)), {0xff}, []byte(strings.Repeat("a", 65537))} {
		os.WriteFile(file, b, 0600)
		if _, e := ReadSetupEnv(file); e == nil {
			t.Fatal("invalid file accepted")
		}
	}
	if _, e := ReadSetupEnv(root); e == nil {
		t.Fatal("directory accepted as env file")
	}
}

func TestEnvCompleteSetupNeedsOnlyFinalConfirmationBeforePersistence(t *testing.T) {
	f := newFixture(t)
	f.o.EnvFile = "fixture.env"
	o, out, e := preview(t, f, "no")
	_ = o
	if e != ErrSetupCancelled || !strings.Contains(out, "非秘密設定ファイル") ||
		!strings.Contains(out, "tunnel_fixture") || exists(f.o.Home) ||
		f.v.ready != 0 || f.r.installs != 0 {
		t.Fatal("preview had side effects", e, out)
	}
}

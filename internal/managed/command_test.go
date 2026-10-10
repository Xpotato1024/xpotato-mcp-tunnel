package managed

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type commandPrompt struct {
	yes   bool
	calls int
}

func (*commandPrompt) Text(string) (string, error) { return "", errors.New("unexpected text") }
func (*commandPrompt) Key() ([]byte, error) {
	return nil, errors.New("command installation cannot collect key")
}
func (p *commandPrompt) Consent(string) (bool, error) { p.calls++; return p.yes, nil }
func TestShortCommandConsentIdempotenceAndUnrelatedFilePreserved(t *testing.T) {
	home := filepath.Join(t.TempDir(), "App With Spaces")
	bin := filepath.Join(t.TempDir(), "User Bin With Spaces")
	p := &commandPrompt{yes: true}
	if e := SetupCommand(home, bin, p); e != nil {
		t.Fatal(e)
	}
	if e := SetupCommand(home, bin, p); e != nil {
		t.Fatal(e)
	}
	if p.calls != 1 {
		t.Fatal("installation consent repeated")
	}
	t.Setenv("PATH", bin)
	if StartCommand(home) != "xpotato-tunnel" {
		t.Fatal("short command not shown")
	}
	r, e := readCommandRecord(home)
	if e != nil {
		t.Fatal(e)
	}
	for file := range r.Files {
		if strings.HasPrefix(file, bin) {
			os.WriteFile(file, []byte("user-customized"), 0600)
			if e = InstallCommand(home); e == nil {
				t.Fatal("unrelated command overwritten")
			}
			b, _ := os.ReadFile(file)
			if string(b) != "user-customized" {
				t.Fatal("modified command not preserved")
			}
			break
		}
	}
}
func TestDeclinedShortCommandDoesNotWriteChosenDirectory(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(t.TempDir(), "not-created")
	p := &commandPrompt{}
	if e := SetupCommand(home, bin, p); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(bin); !os.IsNotExist(e) {
		t.Fatal("declined directory created")
	}
}
func TestInterruptedExecutableStageResumes(t *testing.T) {
	if dest := os.Getenv("XPOTATO_COPY_FIXTURE"); dest != "" {
		source, _ := os.Executable()
		e := stageCopy(source, dest, func() {
			entries, _ := os.ReadDir(filepath.Dir(dest))
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".core-stage-") {
					os.WriteFile(filepath.Join(filepath.Dir(dest), entry.Name()), []byte("partial fixture binary"), 0600)
				}
			}
			os.WriteFile(dest+".ready", nil, 0600)
			for {
				time.Sleep(time.Second)
			}
		})
		if e != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	dest := filepath.Join(t.TempDir(), "core", "xpotato-test")
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "-test.run=^TestInterruptedExecutableStageResumes$")
	cmd.Env = append(os.Environ(), "XPOTATO_COPY_FIXTURE="+dest)
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, e := os.Stat(dest + ".ready"); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("copy helper not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("partial final executable became visible")
	}
	cmd.Process.Kill()
	cmd.Wait()
	if e := stageCopy(exe, dest, nil); e != nil {
		t.Fatal(e)
	}
	a, _ := hashFile(exe)
	b, _ := hashFile(dest)
	if a != b {
		t.Fatal("resumed copy not verified")
	}
}
func TestGeneratedWrapperExecutesNativeArguments(t *testing.T) {
	if os.Getenv("XPOTATO_WRAPPER_FIXTURE") == "1" {
		joined := strings.Join(os.Args, "|")
		if !strings.Contains(joined, "|two words|--home|") {
			os.Exit(2)
		}
		os.Exit(42)
	}
	home := filepath.Join(t.TempDir(), "App With Spaces")
	bin := filepath.Join(t.TempDir(), "Short Bin With Spaces")
	if e := SetupCommand(home, bin, &commandPrompt{yes: true}); e != nil {
		t.Fatal(e)
	}
	if runtime.GOOS == "windows" {
		for _, kind := range []string{"cmd", "ps1"} {
			var cmd *exec.Cmd
			wrapper := filepath.Join(bin, "xpotato-tunnel."+kind)
			if kind == "cmd" {
				cmd = windowsWrapperCommand(wrapper)
			} else {
				cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", wrapper, "-test.run=^TestGeneratedWrapperExecutesNativeArguments$", "--", "two words")
			}
			cmd.Env = append(os.Environ(), "XPOTATO_WRAPPER_FIXTURE=1")
			e := cmd.Run()
			if exit, ok := e.(*exec.ExitError); !ok || exit.ExitCode() != 42 {
				t.Fatalf("%s wrapper argument/exit forwarding failed", kind)
			}
		}
	} else {
		cmd := exec.Command(filepath.Join(bin, "xpotato-tunnel"), "-test.run=^TestGeneratedWrapperExecutesNativeArguments$", "--", "two words")
		cmd.Env = append(os.Environ(), "XPOTATO_WRAPPER_FIXTURE=1")
		e := cmd.Run()
		if exit, ok := e.(*exec.ExitError); !ok || exit.ExitCode() != 42 {
			t.Fatal("sh wrapper argument/exit forwarding failed")
		}
	}
}

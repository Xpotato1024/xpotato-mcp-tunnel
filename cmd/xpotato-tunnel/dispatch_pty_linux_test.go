//go:build linux

package main

import (
	"context"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/managed"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLinuxDispatchPreservesForegroundInputAndPromptInterrupt(t *testing.T) {
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Skip("Python pty fixture unavailable")
	}
	home := t.TempDir()
	if e = managed.SetupCommand(home, "", dispatchPrompt{}); e != nil {
		t.Fatal(e)
	}
	p, e := managed.PrepareCommandUpdate(context.Background(), home)
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(); e != nil {
		t.Fatal(e)
	}
	p.Close()
	const script = `import errno, os, pty, select, signal, sys, time
pid, fd = pty.fork()
if pid == 0:
    os.execve(sys.argv[1], [sys.argv[1], 'setup'], os.environ)
output, sent, waited = b'', False, False
deadline = time.monotonic() + 12
try:
    while time.monotonic() < deadline:
        ready, _, _ = select.select([fd], [], [], .05)
        if ready:
            try:
                chunk = os.read(fd, 4096)
                if not chunk: break
                output += chunk
            except OSError as error:
                if error.errno != errno.EIO: raise
                break
        if not sent and b'dispatch input fixture' in output:
            os.write(fd, b'\x03' if sys.argv[2] == 'interrupt' else b'synthetic input\n')
            sent = True
    else:
        raise AssertionError('PTY child timed out (possible SIGTTIN or swallowed Ctrl+C): ' + repr(output))
    _, status = os.waitpid(pid, 0)
    waited = True
    assert sent, repr(output)
    if sys.argv[2] == 'input':
        assert os.waitstatus_to_exitcode(status) == 42 and b'TEXT_OK|synthetic input' in output, repr(output)
    else:
        assert b'TEXT_OK' not in output and os.waitstatus_to_exitcode(status) != 0, repr(output)
finally:
    if not waited:
        try: os.kill(pid, signal.SIGKILL)
        except ProcessLookupError: pass
        os.waitpid(pid, 0)
    os.close(fd)
`
	for _, mode := range []string{"input", "interrupt"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(python, "-c", script, filepath.Join(home, "bin", "xpotato-tunnel"), mode)
			cmd.Env = append(os.Environ(), "XPOTATO_DISPATCH_FIXTURE=1")
			if output, e := cmd.CombinedOutput(); e != nil {
				t.Fatalf("PTY regression: %v; %s", e, output)
			}
		})
	}
}

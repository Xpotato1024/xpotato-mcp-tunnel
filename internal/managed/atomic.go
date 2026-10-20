package managed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/atomicfile"
	"io"
	"os"
	"path/filepath"
)

// Copy through an owned same-directory stage; the final executable is never
// visible partially written. A process killed mid-copy leaves only a stage.
func stageCopy(source, destination string, ready func()) error {
	if e := os.MkdirAll(filepath.Dir(destination), 0700); e != nil {
		return e
	}
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.CreateTemp(filepath.Dir(destination), ".core-stage-")
	if e != nil {
		return e
	}
	defer os.Remove(out.Name())
	if e = out.Chmod(0700); e == nil {
		if ready != nil {
			ready()
		}
		_, e = io.Copy(out, in)
	}
	if e == nil {
		e = out.Sync()
	}
	ce := out.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	expected, e := hashFile(source)
	if e != nil {
		return e
	}
	actual, e := hashFile(out.Name())
	if e != nil {
		return e
	}
	if expected != actual {
		return io.ErrUnexpectedEOF
	}
	return os.Rename(out.Name(), destination)
}
func atomicBytes(file string, b []byte) error {
	return atomicfile.Write(context.Background(), file, b, 0600)
}

// The source preflight hash is the expected identity. Hash while copying and
// reread the staged bytes: source mutation cannot silently change that identity.
func stageCopyChecked(ctx context.Context, source, destination, expected string, ready func()) error {
	ctx, cancel := context.WithTimeout(ctx, fileOperationTimeout)
	defer cancel()
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Dir(destination), 0700); e != nil {
		return e
	}
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.CreateTemp(filepath.Dir(destination), ".core-stage-")
	if e != nil {
		return e
	}
	defer os.Remove(out.Name())
	defer out.Close()
	if e = out.Chmod(0700); e != nil {
		return e
	}
	if ready != nil {
		ready()
	}
	h := sha256.New()
	if _, e = io.Copy(io.MultiWriter(out, h), contextReader{ctx, in}); e != nil {
		return e
	}
	if hex.EncodeToString(h.Sum(nil)) != expected {
		return io.ErrUnexpectedEOF
	}
	if e = out.Sync(); e != nil {
		return e
	}
	if e = out.Close(); e != nil {
		return e
	}
	actual, e := hashFileContext(ctx, out.Name())
	if e != nil {
		return e
	}
	if actual != expected {
		return io.ErrUnexpectedEOF
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return os.Rename(out.Name(), destination)
}

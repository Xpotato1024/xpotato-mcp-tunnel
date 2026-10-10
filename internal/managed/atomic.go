package managed

import (
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
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(file), ".command-stage-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), file)
}

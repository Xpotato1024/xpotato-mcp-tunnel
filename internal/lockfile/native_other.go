//go:build !windows && !linux && !darwin

package lockfile

import "os"

func openLock(string) (*os.File, error) { return nil, ErrOwner }
func ownedFile(*os.File) error          { return ErrOwner }
func tryLock(*os.File) error            { return ErrOwner }
func unlock(*os.File)                   {}

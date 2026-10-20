package core

import (
	"context"
	"encoding/hex"
	"errors"
	"regexp"
)

// CorePin and Device.Release are published in the same device.json replacement.
// They are consistency metadata, not a signature or an AV trust decision.
type CorePin struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

var coreVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func (p CorePin) Valid() bool {
	b, e := hex.DecodeString(p.SHA256)
	return coreVersionPattern.MatchString(p.Version) && len(b) == 32 && e == nil
}

type CommandUpdate interface {
	Current() CorePin
	Legacy() CorePin
	Migrate() error
	Verify() error
	Close()
}

// PreparedRuntime owns the installation lock and only its own temporary stage.
// Verify performs a full inventory/hash check immediately before publication.
// A returned id is never a persisted "verified" cache flag.
type PreparedRuntime interface {
	Verify() (string, error)
	Close()
}
type UpdateRuntime interface {
	PrepareRuntime(home, current string) (PreparedRuntime, error)
}

func (a App) context() context.Context {
	if a.Context != nil {
		return a.Context
	}
	return context.Background()
}
func (a App) saveDevice(o Options, d Device) error {
	if a.writeDevice != nil {
		return a.writeDevice(deviceFile(o), d)
	}
	return atomicJSONContext(a.context(), deviceFile(o), d)
}

func (a App) updateGeneration(o Options, d Device, rollback bool) error {
	ctx := a.context()
	if err := ctx.Err(); err != nil {
		return err
	}
	if rollback {
		if d.Previous == "" {
			return errors.New("no previous runtime")
		}
		if d.Core != nil && d.PreviousCore == nil {
			return errors.New("previous core generation unavailable; current generation preserved")
		}
		if d.PreviousCore != nil && a.VerifyCore != nil {
			if err := a.VerifyCore(ctx, o.Home, *d.PreviousCore); err != nil {
				return err
			}
		}
		if err := a.Runtime.Verify(o.Home, d.Previous); err != nil {
			return err
		}
		d.Release, d.Previous = d.Previous, d.Release
		d.Core, d.PreviousCore = d.PreviousCore, d.Core
		return a.saveDevice(o, d)
	}
	// The source executable must be readable/copyable before network/npm work.
	command, err := a.PrepareUpdate(ctx, o.Home)
	if err != nil {
		return err
	}
	defer command.Close()
	nextCore := command.Current()
	if !nextCore.Valid() {
		return errors.New("prepared core pin invalid")
	}
	var prepared PreparedRuntime
	var next string
	if r, ok := a.Runtime.(UpdateRuntime); ok {
		prepared, err = r.PrepareRuntime(o.Home, d.Release)
		if err != nil {
			return err
		}
		defer prepared.Close()
	} else {
		next, err = a.Runtime.Install(o.Home)
		if err != nil {
			return err
		}
	}
	if d.Core == nil {
		legacy := command.Legacy()
		if !legacy.Valid() {
			return errors.New("legacy core generation unavailable; existing entry points preserved")
		}
		d.Core = &legacy
		// Mixed old/stable wrappers still select this complete old generation.
		if err = a.saveDevice(o, d); err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = command.Migrate(); err != nil {
		return err
	}
	if prepared != nil {
		next, err = prepared.Verify()
	} else {
		err = a.Runtime.Verify(o.Home, next)
	}
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = command.Verify(); err != nil {
		return err
	}
	if d.Release == next && *d.Core == nextCore {
		return nil
	}
	d.Previous, d.Release = d.Release, next
	d.PreviousCore, d.Core = d.Core, &nextCore
	return a.saveDevice(o, d)
}

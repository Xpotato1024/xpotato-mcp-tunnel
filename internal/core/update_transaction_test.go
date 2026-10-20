package core

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

type fixtureCommandUpdate struct {
	current, legacy CorePin
	migrate         func() error
	closed          bool
}

func (p *fixtureCommandUpdate) Current() CorePin { return p.current }
func (p *fixtureCommandUpdate) Legacy() CorePin  { return p.legacy }
func (p *fixtureCommandUpdate) Verify() error    { return nil }
func (p *fixtureCommandUpdate) Migrate() error {
	if p.migrate != nil {
		return p.migrate()
	}
	return nil
}
func (p *fixtureCommandUpdate) Close() { p.closed = true }

func transactionFixture(t *testing.T) (fixture, *fixtureCommandUpdate) {
	t.Helper()
	f := prepared(t)
	p := &fixtureCommandUpdate{current: CorePin{Version, strings.Repeat("a", 64)}, legacy: CorePin{"0.2.6", strings.Repeat("b", 64)}}
	f.app.PrepareUpdate = func(context.Context, string) (CommandUpdate, error) { return p, nil }
	return f, p
}

func TestUpdateActivatesCoreAndRuntimeTogetherAndRollsBack(t *testing.T) {
	f, p := transactionFixture(t)
	old, _ := f.app.owned(f.o)
	f.r.next = "release-two"
	if err := f.app.Update(f.o, false); err != nil {
		t.Fatal(err)
	}
	d, _ := f.app.owned(f.o)
	if d.Release != "release-two" || d.Core == nil || *d.Core != p.current || d.PreviousCore == nil || *d.PreviousCore != p.legacy || d.Previous != old.Release {
		t.Fatal("incomplete generation", d)
	}
	d.Core, d.PreviousCore = nil, nil
	d.Release, d.Previous = old.Release, old.Previous
	if !reflect.DeepEqual(d, old) {
		t.Fatal("scope/credential state changed")
	}
	if !p.closed || f.v.loads != 0 {
		t.Fatal("preparation leak or vault read")
	}
	if err := f.app.Update(f.o, true); err != nil {
		t.Fatal(err)
	}
	d, _ = f.app.owned(f.o)
	if d.Release != old.Release || *d.Core != p.legacy || *d.PreviousCore != p.current {
		t.Fatal("rollback mixed generation")
	}
}

func TestCoreOnlyUpdateStillHasPreviousGeneration(t *testing.T) {
	f, p := transactionFixture(t)
	old, _ := f.app.owned(f.o)
	if err := f.app.Update(f.o, false); err != nil {
		t.Fatal(err)
	}
	d, _ := f.app.owned(f.o)
	if d.Release != old.Release || d.Previous != old.Release || *d.Core != p.current || *d.PreviousCore != p.legacy {
		t.Fatal("core-only activation lost rollback")
	}
}

func TestUpdatePreparationFailsBeforeRuntimeWork(t *testing.T) {
	f, _ := transactionFixture(t)
	before, _ := os.ReadFile(deviceFile(f.o))
	installs := f.r.installs
	f.app.PrepareUpdate = func(context.Context, string) (CommandUpdate, error) {
		return nil, errors.New("fixture source unreadable")
	}
	if err := f.app.Update(f.o, false); err == nil {
		t.Fatal("source failure ignored")
	}
	after, _ := os.ReadFile(deviceFile(f.o))
	if string(before) != string(after) || f.r.installs != installs {
		t.Fatal("runtime/state changed before core preflight")
	}
}

func TestUpdateFailureAndCancellationPreserveOldGeneration(t *testing.T) {
	for _, stage := range []string{"migrate", "cancel", "save"} {
		t.Run(stage, func(t *testing.T) {
			f, p := transactionFixture(t)
			old, _ := f.app.owned(f.o)
			f.r.next = "release-two"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.app.Context = ctx
			p.migrate = func() error {
				if stage == "cancel" {
					cancel()
				}
				if stage == "migrate" {
					return errors.New("fixture wrapper write")
				}
				return nil
			}
			if stage == "save" {
				f.app.writeDevice = func(path string, d Device) error {
					if d.Release == "release-two" {
						return errors.New("fixture activation write")
					}
					return atomicJSON(path, d)
				}
			}
			if err := f.app.Update(f.o, false); err == nil {
				t.Fatal("failure ignored")
			}
			d, _ := f.app.owned(f.o)
			if d.Release != old.Release || (d.Core != nil && *d.Core != p.legacy) || f.v.loads != 0 || !p.closed {
				t.Fatal("old generation lost")
			}
		})
	}
}

func TestStartRejectsSelectedOtherCoreBeforeVault(t *testing.T) {
	f := prepared(t)
	d, _ := f.app.owned(f.o)
	d.Core = &CorePin{"0.2.6", strings.Repeat("b", 64)}
	if err := atomicJSON(deviceFile(f.o), d); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.Start(f.o); err == nil || f.v.loads != 0 || f.r.runs != 0 {
		t.Fatal("wrong core used saved key")
	}
}

type cancellingVault struct {
	*fakeVault
	cancel context.CancelFunc
	loaded []byte
}

func (v *cancellingVault) Load(ref string) ([]byte, error) {
	key, e := v.fakeVault.Load(ref)
	v.loaded = key
	v.cancel()
	return key, e
}
func TestStartCancelledDuringSyntheticVaultReadDoesNotLaunch(t *testing.T) {
	f := prepared(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v := &cancellingVault{fakeVault: f.v, cancel: cancel}
	f.app.Context = ctx
	f.app.Vault = v
	if _, e := f.app.Start(f.o); !errors.Is(e, context.Canceled) || f.r.runs != 0 {
		t.Fatal("cancelled Start launched", e)
	}
	for _, b := range v.loaded {
		if b != 0 {
			t.Fatal("cancelled key buffer retained")
		}
	}
}

type fixtureRuntimePreparation struct {
	verify func() (string, error)
	closed bool
}

func (p *fixtureRuntimePreparation) Verify() (string, error) { return p.verify() }
func (p *fixtureRuntimePreparation) Close()                  { p.closed = true }

type transactionRuntime struct {
	*fakeRuntime
	prepared              *fixtureRuntimePreparation
	legacyCalls, prepares int
}

func (r *transactionRuntime) PrepareRuntime(string, string) (PreparedRuntime, error) {
	r.prepares++
	return r.prepared, nil
}
func (r *transactionRuntime) Install(string) (string, error) {
	r.legacyCalls++
	return "", errors.New("legacy Install should not run")
}
func (r *transactionRuntime) Verify(string, string) error {
	r.legacyCalls++
	return errors.New("duplicate legacy Verify should not run")
}

func TestUpdatePreparedRuntimeMainPathClosesAndDoesNotDuplicateVerification(t *testing.T) {
	for _, mode := range []string{"success", "verify-failure", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f, command := transactionFixture(t)
			old, _ := f.app.owned(f.o)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.app.Context = ctx
			calls := 0
			prep := &fixtureRuntimePreparation{verify: func() (string, error) {
				calls++
				if mode == "verify-failure" {
					return "", errors.New("fixture full verification failed")
				}
				if mode == "cancel" {
					cancel()
				}
				return "release-two", nil
			}}
			r := &transactionRuntime{fakeRuntime: f.r, prepared: prep}
			f.app.Runtime = r
			err := f.app.Update(f.o, false)
			if (err == nil) != (mode == "success") {
				t.Fatal("wrong activation result", err)
			}
			d, _ := f.app.owned(f.o)
			if mode == "success" {
				if d.Release != "release-two" || *d.Core != command.current {
					t.Fatal("generation not activated")
				}
			} else {
				if d.Release != old.Release || *d.Core != command.legacy {
					t.Fatal("failed preparation changed generation")
				}
			}
			if calls != 1 || r.prepares != 1 || r.legacyCalls != 0 || !prep.closed || !command.closed || f.v.loads != 0 {
				t.Fatal("duplicate verification/resource leak/vault access")
			}
		})
	}
}

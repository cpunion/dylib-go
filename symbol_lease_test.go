package dylib

import (
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	examplecall "github.com/cpunion/dylib-go/examples/call"
)

func TestSymbolLeaseValidationAndIndependentRetention(t *testing.T) {
	for _, symbol := range []*Symbol{nil, {}} {
		if _, err := symbol.Acquire(); !errors.Is(err, ErrClosed) {
			t.Fatal("unbound acquisition:", err)
		}
	}
	for _, lease := range []*SymbolLease{nil, {}} {
		if _, err := lease.Address(); !errors.Is(err, ErrClosed) {
			t.Fatal("unbound address:", err)
		}
		if err := lease.WithAddress(nil); !errors.Is(err, ErrClosed) {
			t.Fatal("unbound use:", err)
		}
		if err := lease.Close(); err != nil {
			t.Fatal(err)
		}
	}
	session := New(Options{})
	defer session.Close()
	symbol := &Symbol{owner: session, address: 0x1234}
	one, err := symbol.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	two, err := symbol.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer two.Close()
	if err := one.WithAddress(nil); err == nil {
		t.Fatal("nil use accepted")
	}
	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()
	waitSessionRetired(t, session)
	if _, err := symbol.Acquire(); !errors.Is(err, ErrClosed) {
		t.Fatal("retired acquisition:", err)
	}
	if address, err := one.Address(); address != 0x1234 || err != nil {
		t.Fatal("retained address:", address, err)
	}
	one.Close()
	select {
	case <-closed:
		t.Fatal("session ignored independent second lease")
	default:
	}
	if err := two.WithAddress(func(address uintptr) error {
		return two.WithAddress(func(inner uintptr) error {
			if inner != address {
				t.Error("reentrant address changed")
			}
			return nil
		})
	}); err != nil {
		t.Fatal("retained reentry:", err)
	}
	two.Close()
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if _, err := two.Address(); !errors.Is(err, ErrClosed) {
		t.Fatal("released address:", err)
	}
}

func TestSymbolLeaseConcurrentUseAndRetirement(t *testing.T) {
	session := New(Options{})
	defer session.Close()
	lease, err := (&Symbol{owner: session, address: 1}).Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- lease.WithAddress(func(uintptr) error {
				entered <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("lease invocations were serialized")
		}
	}
	closed := make(chan error, 2)
	go func() { closed <- lease.Close() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		lease.mu.Lock()
		retired := lease.owner == nil
		lease.mu.Unlock()
		if retired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lease did not retire")
		}
		runtime.Gosched()
	}
	go func() { closed <- lease.Close() }()
	if err := lease.WithAddress(func(uintptr) error { t.Error("retired lease invoked adapter"); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	select {
	case <-closed:
		t.Fatal("lease released during an active adapter")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
		if err := <-closed; err != nil {
			t.Fatal(err)
		}
	}
}

func TestSymbolLeasePanicReleasesUse(t *testing.T) {
	session := New(Options{})
	defer session.Close()
	lease, err := (&Symbol{owner: session, address: 1}).Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	want := errors.New("adapter failure")
	if err := lease.WithAddress(func(uintptr) error { return want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() != "adapter panic" {
				t.Error("panic not preserved")
			}
		}()
		_ = lease.WithAddress(func(uintptr) error { panic("adapter panic") })
	}()
	lease.Close()
}

func TestSymbolLeaseNativeCallDuringSessionRetirement(t *testing.T) {
	needNative(t)
	session := New(Options{})
	defer session.Close()
	load(t, session, compile(t, "testdata/add.c", filepath.Join(t.TempDir(), "add.o")))
	symbol, err := session.Resolve("add")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := symbol.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()
	waitSessionRetired(t, session)
	if err := symbol.WithAddress(func(uintptr) error { t.Error("retired symbol invoked"); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err := lease.WithAddress(func(address uintptr) error {
		got, err := examplecall.Int32(address, 20, 22)
		if got != 42 {
			t.Error("retained native call:", got)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	lease.Close()
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}

func TestRegistrationWithNativeSymbolAdapterWithoutLibffi(t *testing.T) {
	needNative(t)
	session := New(Options{})
	defer session.Close()
	load(t, session, compile(t, "testdata/add.c", filepath.Join(t.TempDir(), "add.o")))
	symbol, err := session.Resolve("add")
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(leases RegistrationLeases) error {
		return leases.Symbols[0].WithAddress(func(address uintptr) error {
			got, err := examplecall.Int32(address, 20, 22)
			if got != 42 {
				t.Error("static adapter retained call:", got)
			}
			return err
		})
	}
	r, err := NewRegistration(RegistrationResources{Symbols: []*Symbol{symbol}}, invoke)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.WithLeases(invoke); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()
	waitSessionRetired(t, session)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}

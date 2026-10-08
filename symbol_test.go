package dylib

import (
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestSymbolAccessErrors(t *testing.T) {
	for _, symbol := range []*Symbol{nil, {}} {
		if err := symbol.WithAddress(func(uintptr) error { return nil }); !errors.Is(err, ErrClosed) {
			t.Fatalf("unbound symbol: %v", err)
		}
	}
	needNative(t)
	s := New(Options{})
	defer s.Close()
	load(t, s, compile(t, "testdata/add.c", filepath.Join(t.TempDir(), "add.o")))
	symbol, err := s.Resolve("add")
	if err != nil {
		t.Fatal(err)
	}
	if err = symbol.WithAddress(nil); err == nil {
		t.Fatal("nil callback accepted")
	}
	want := errors.New("adapter failure")
	if err = symbol.WithAddress(func(address uintptr) error {
		if address == 0 {
			t.Fatal("resolved address is zero")
		}
		return want
	}); !errors.Is(err, want) {
		t.Fatalf("callback error: %v", err)
	}
	// An adapter failure must release its lifetime guard without closing it.
	call(t, s, "add", 20, 22, 42)
}

func TestSymbolReentrantAndConcurrentUse(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	symbol := &Symbol{owner: s, address: 0x1234}
	if err := symbol.WithAddress(func(address uintptr) error {
		if err := s.Define("marker", address); err != nil {
			return err
		}
		return symbol.WithAddress(func(inner uintptr) error {
			if inner != address {
				t.Errorf("nested address: %#x", inner)
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- symbol.WithAddress(func(uintptr) error {
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
			t.Fatal("address users were serialized")
		}
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestSymbolPanicReleasesLifetime(t *testing.T) {
	s := New(Options{})
	symbol := &Symbol{owner: s, address: 1}
	func() {
		defer func() {
			if recover() != "adapter panic" {
				t.Error("adapter panic was not preserved")
			}
		}()
		_ = symbol.WithAddress(func(uintptr) error { panic("adapter panic") })
	}()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func waitSessionRetired(t *testing.T, s *Session) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("session did not begin retirement")
		}
		runtime.Gosched()
	}
}

func TestSymbolRetirementRejectsReentry(t *testing.T) {
	s := New(Options{})
	symbol := &Symbol{owner: s, address: 1}
	entered, release := make(chan struct{}), make(chan struct{})
	used, closed := make(chan error, 1), make(chan error, 2)
	go func() {
		used <- symbol.WithAddress(func(uintptr) error {
			close(entered)
			<-release
			return symbol.WithAddress(func(uintptr) error {
				return errors.New("retired session started nested address use")
			})
		})
	}()
	<-entered
	go func() { closed <- s.Close() }()
	waitSessionRetired(t, s)
	go func() { closed <- s.Close() }()
	select {
	case <-closed:
		close(release)
		t.Fatal("Close returned before active use completed")
	default:
	}
	close(release)
	if err := <-used; !errors.Is(err, ErrClosed) {
		t.Fatalf("reentry during retirement: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := <-closed; err != nil {
			t.Fatal(err)
		}
	}
}

func TestSymbolAccessRetainsSession(t *testing.T) {
	needNative(t)
	s := New(Options{})
	defer s.Close()
	load(t, s, compile(t, "testdata/add.c", filepath.Join(t.TempDir(), "add.o")))
	symbol, err := s.Resolve("add")
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	used, closed := make(chan error, 1), make(chan error, 1)
	go func() {
		used <- symbol.WithAddress(func(uintptr) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	closeStarted := make(chan struct{})
	go func() {
		close(closeStarted)
		closed <- s.Close()
	}()
	<-closeStarted
	select {
	case err := <-closed:
		close(release)
		t.Fatalf("Close completed during symbol access: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-used; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := symbol.WithAddress(func(uintptr) error {
		t.Fatal("closed symbol invoked adapter")
		return nil
	}); !errors.Is(err, ErrClosed) {
		t.Fatalf("access after close: %v", err)
	}
}

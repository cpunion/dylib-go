package dylib

import (
	"errors"
	"path/filepath"
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
	// An adapter failure must release the session lock without closing it.
	call(t, s, "add", 20, 22, 42)
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

package dylib

import (
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestCloseLibrariesRetainsFailedHandles(t *testing.T) {
	failure := errors.New("native close failed")
	var released []uintptr
	fail := map[uintptr]bool{2: true, 4: true}
	release := func(handle uintptr) error {
		released = append(released, handle)
		if fail[handle] {
			return failure
		}
		return nil
	}
	pending, err := closeLibraries([]uintptr{1, 2, 3, 4}, release)
	if !errors.Is(err, failure) || !reflect.DeepEqual(pending, []uintptr{2, 4}) || !reflect.DeepEqual(released, []uintptr{4, 3, 2, 1}) {
		t.Fatal(pending, released, err)
	}
	delete(fail, 2)
	released = nil
	pending, err = closeLibraries(pending, release)
	if !errors.Is(err, failure) || !reflect.DeepEqual(pending, []uintptr{4}) || !reflect.DeepEqual(released, []uintptr{4, 2}) {
		t.Fatal("retried an already released library", pending, released, err)
	}
	delete(fail, 4)
	released = nil
	pending, err = closeLibraries(pending, release)
	if err != nil || len(pending) != 0 || !reflect.DeepEqual(released, []uintptr{4}) {
		t.Fatal(pending, released, err)
	}
}

func closeRetrySession(t *testing.T) (*Session, *Session, *image, func(int32, int32) (int32, error)) {
	t.Helper()
	needNative(t)
	dir := t.TempDir()
	host, _, read := lifecycleObserver(t, dir)
	provider := nativeInputSession(t, "testdata/add.c", "object")
	add, err := provider.Resolve("add")
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{EmulatedTLS: true})
	load(t, s, host)
	for _, part := range []string{"consumer", "provider"} {
		load(t, s, compile(t, "testdata/emutls_"+part+".c", filepath.Join(dir, part+".o"), "-femulated-tls"))
	}
	load(t, s, compile(t, "testdata/close_retry.c", filepath.Join(dir, "cleanup.o")))
	if err := s.DefineSymbol("retained_add", add); err != nil {
		t.Fatal(err)
	}
	call(t, s, "close_retry_register", 20, 22, 42)
	call(t, s, "tls_step", 20, 22, 49)
	im := s.image
	if im.emulatedTLS.LiveValues() != 3 {
		t.Fatal("TLS fixture did not allocate three values")
	}
	t.Cleanup(func() {
		if im.unwind != nil {
			im.unwind.dynamic = 0
		}
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, provider, im, read
}

func TestNativeCloseRetainsResourcesForRetry(t *testing.T) {
	needNative(t)
	for _, mode := range []string{"single", "concurrent"} {
		t.Run(mode, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			s, provider, im, read := closeRetrySession(t)
			// This native API rejects a missing cache invalidator before touching
			// a handle on macOS, and rejects the platform on Linux/Windows.
			// It gives every host a deterministic, non-destructive cleanup error.
			im.unwind = &runtimeFunctions{dynamic: 1}
			step, err := s.Resolve("tls_step")
			if err != nil {
				t.Fatal(err)
			}
			lease, err := step.Acquire()
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			count := 1
			if mode == "concurrent" {
				count = 8
			}
			closed := make(chan error, count)
			go func() { closed <- s.Close() }()
			waitSessionRetired(t, s)
			for i := 1; i < count; i++ {
				go func() { closed <- s.Close() }()
			}
			select {
			case <-closed:
				t.Fatal("close ignored an active code lease")
			default:
			}
			lease.Close()
			for i := 0; i < count; i++ {
				select {
				case err := <-closed:
					if err == nil {
						t.Fatal("native cleanup failure was lost")
					}
				case <-time.After(5 * time.Second):
					t.Fatal("concurrent close did not finish")
				}
			}
			if _, err := s.Resolve("tls_step"); !errors.Is(err, ErrClosed) {
				t.Fatal("failed close reopened the owner", err)
			}
			if err := step.WithAddress(func(uintptr) error { t.Fatal("retired code was called"); return nil }); !errors.Is(err, ErrClosed) {
				t.Fatal(err)
			}
			if s.failedImage != im || len(s.libs) != 1 || len(s.symbolDependencies) != 1 || len(im.mem) == 0 || im.emulatedTLS.LiveValues() != 3 {
				t.Fatal("failed close discarded image, libraries, dependencies or TLS")
			}
			expectEvents(t, read, 42, 3)
			if err := s.Close(); err == nil {
				t.Fatal("sequential retry reported success before native cleanup succeeded")
			}
			expectEvents(t, read, 42, 3)
			providerClosed := make(chan error, 1)
			go func() { providerClosed <- provider.Close() }()
			waitSessionRetired(t, provider)
			select {
			case <-providerClosed:
				t.Fatal("provider released before consumer cleanup succeeded")
			default:
			}
			im.unwind.dynamic = 0 // Simulate recovery of the native unregister operation.
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-providerClosed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("successful retry retained provider lease")
			}
			if im.mem != nil || im.emulatedTLS.LiveValues() != 0 || s.failedImage != nil || len(s.libs) != 0 || len(s.symbolDependencies) != 0 {
				t.Fatal("successful retry retained native resources")
			}
			if err := s.Close(); err != nil {
				t.Fatal("completed close was not idempotent", err)
			}
			expectEvents(t, read, 42, 3)
		})
	}
}

func TestNativeCloseRetriesImageMemoryRelease(t *testing.T) {
	needNative(t)
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("syscall.Munmap validates the original complete slice on POSIX")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s, _, im, read := closeRetrySession(t)
	mem := im.mem
	defer func() {
		if im.mem != nil {
			im.mem = mem
		}
		s.Close()
	}()
	// The native mapper rejects an incomplete slice without unmapping pages.
	// Restore it for the retry; no OS function receives an invalid address.
	im.mem = mem[:len(mem)-1]
	if err := s.Close(); err == nil {
		t.Fatal("incomplete native mapping was released")
	}
	if im.mem == nil || s.failedImage != im || len(s.libs) != 1 || len(s.symbolDependencies) != 1 {
		t.Fatal("memory release failure lost ownership")
	}
	if im.emulatedTLS.LiveValues() != 0 {
		t.Fatal("completed TLS cleanup was deferred unnecessarily")
	}
	expectEvents(t, read, 42, 3)
	im.mem = mem
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if im.mem != nil || s.failedImage != nil {
		t.Fatal("memory release retry did not finish")
	}
	expectEvents(t, read, 42, 3)
}

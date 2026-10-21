package dylib

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ramblelabs/dylib-go/abi"
	examplecall "github.com/ramblelabs/dylib-go/examples/call"
)

func initializationCallback(t *testing.T, handler abi.CallbackFunc) (*abi.Callback, uintptr) {
	t.Helper()
	needNative(t)
	if !abi.Available() {
		t.Skip("requires -tags libffi for the test callback")
	}
	cb, err := abi.NewCallback(abi.Signature{Args: []abi.Type{abi.I32}}, handler)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cb.Close() })
	lease, err := cb.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lease.Close() })
	address, err := lease.Address()
	if err != nil {
		t.Fatal(err)
	}
	return cb, address
}

func initializationBusy(s *Session, name, path string) error {
	for _, operation := range []func() error{
		func() error { return s.Link(name) },
		func() error { _, err := s.Lookup(name); return err },
		func() error { _, err := s.Resolve(name); return err },
		func() error { _, err := s.Bind(name, abi.Signature{}); return err },
		func() error { return s.Load(path) },
		func() error { return s.Define("blocked_definition", 1) },
	} {
		if err := operation(); !errors.Is(err, ErrInitializing) {
			return fmt.Errorf("initializing operation: %v", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.image != nil || len(s.plans) != 0 || s.defined["blocked_definition"] != 0 {
		return fmt.Errorf("initializing operation published resources")
	}
	return nil
}

func initializationSum(s *Session, name string, want int32) error {
	return s.WithInitializingAddress(name, func(address uintptr) error {
		got, err := examplecall.Int32(address, 20, 22)
		if err == nil && got != want {
			err = fmt.Errorf("scoped %s: %d, want %d", name, got, want)
		}
		return err
	})
}

func TestNativeInitializerReentry(t *testing.T) {
	for _, input := range []string{"object", "archive"} {
		t.Run(input, func(t *testing.T) {
			s := New(Options{})
			var events []int32
			var path string
			if !abi.Available() {
				t.Skip("requires -tags libffi")
			}
			plan, err := abi.Prepare(abi.Signature{Args: []abi.Type{abi.I32}})
			if err != nil {
				t.Fatal(err)
			}
			defer plan.Close()
			cb, address := initializationCallback(t, func(args []abi.Value) (abi.Value, error) {
				event := int32(args[0].Bits)
				events = append(events, event)
				if event >= 5 && event != 42 {
					err := s.WithInitializingAddress("array_root", func(uintptr) error { return nil })
					if !errors.Is(err, ErrClosed) {
						return abi.Value{}, fmt.Errorf("finalizer scope: %v", err)
					}
					return abi.Value{}, nil
				}
				if err := initializationBusy(s, "array_root", path); err != nil {
					return abi.Value{}, err
				}
				if err := initializationSum(s, "array_root", 42); err != nil {
					return abi.Value{}, err
				}
				if event == 3 {
					// Go -> native initializer -> Go -> C entry -> Go -> raw C.
					err := s.WithInitializingAddress("record_event", func(p uintptr) error {
						_, err := plan.Call(p, abi.Int32(42))
						return err
					})
					return abi.Value{}, err
				}
				return abi.Value{}, nil
			})
			defer s.Close() // The callback lease outlives native finalization.
			if err := s.Define("record_event", address); err != nil {
				t.Fatal(err)
			}
			path = compile(t, "testdata/lifecycle_arrays.c", filepath.Join(t.TempDir(), "arrays.o"))
			if input == "archive" {
				archive := filepath.Join(t.TempDir(), "arrays.a")
				command(t, "ar", "rcs", archive, path)
				path = archive
			}
			load(t, s, path)
			call(t, s, "array_root", 20, 22, 42)
			if err := s.WithInitializingAddress("array_root", func(uintptr) error { return nil }); !errors.Is(err, ErrNotInitializing) {
				t.Fatalf("scope after publication: %v", err)
			}
			if err := s.Close(); err != nil || cb.Err() != nil {
				t.Fatalf("initializer reentry: %v, callback=%v", err, cb.Err())
			}
			want := []int32{3, 42, 4, 5, 6}
			if runtime.GOOS == "linux" {
				want = append([]int32{0}, want...)
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("initializer events: %v, want %v", events, want)
			}
		})
	}
}

func initializationLibrary(t *testing.T, address uintptr) string {
	t.Helper()
	name, flag := "initialization.so", "-shared"
	if runtime.GOOS == "darwin" {
		name, flag = "initialization.dylib", "-dynamiclib"
	} else if runtime.GOOS == "windows" {
		name = "initialization.dll"
	}
	path := filepath.Join(t.TempDir(), name)
	args := []string{flag, "-O0", "testdata/initialization_library.c", "-o", path, fmt.Sprintf("-DHOOK_ADDRESS=0x%xULL", address)}
	if runtime.GOOS == "windows" {
		args = append(args, "-Wl,--out-implib,"+filepath.Join(filepath.Dir(path), "initialization.a"))
	}
	if runtime.GOOS != "windows" {
		args = append(args, "-fPIC")
	}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		args = append(args, "--target=x86_64-apple-macosx11")
	}
	command(t, compiler(), args...)
	return path
}

func TestNativeLibraryInitializerReentry(t *testing.T) {
	inputs := []string{"library"}
	if runtime.GOOS == "windows" {
		inputs = append(inputs, "import archive")
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) { testNativeLibraryInitializerReentry(t, input) })
	}
}

func testNativeLibraryInitializerReentry(t *testing.T, input string) {
	s := New(Options{})
	var path string
	var events []int32
	cb, address := initializationCallback(t, func(args []abi.Value) (abi.Value, error) {
		event := int32(args[0].Bits)
		events = append(events, event)
		if event == 5 {
			if err := initializationSum(s, "add", 42); !errors.Is(err, ErrClosed) {
				return abi.Value{}, fmt.Errorf("library finalizer: %v", err)
			}
			return abi.Value{}, nil
		}
		if err := initializationBusy(s, "initialization_root", path); err != nil {
			return abi.Value{}, err
		}
		if err := initializationSum(s, "add", 42); err != nil {
			return abi.Value{}, err
		}
		if err := initializationSum(s, "explicit_add", 42); err != nil {
			return abi.Value{}, err
		}
		if err := s.WithInitializingAddress("initialization_root", func(uintptr) error { return nil }); err == nil {
			return abi.Value{}, fmt.Errorf("unpublished library symbol resolved")
		}
		return abi.Value{}, nil
	})
	provider := New(Options{})
	defer provider.Close()
	defer s.Close() // Release consumer dependencies before closing the provider.
	lib := compileSharedAdd(t, t.TempDir())
	load(t, provider, lib)
	symbol, err := provider.Resolve("add")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DefineSymbol("explicit_add", symbol); err != nil {
		t.Fatal(err)
	}
	load(t, s, lib)
	path = initializationLibrary(t, address)
	if input == "import archive" {
		path = filepath.Join(filepath.Dir(path), "initialization.a")
	}
	load(t, s, path)
	call(t, s, "initialization_root", 20, 22, 42)
	if err := s.Close(); err != nil || cb.Err() != nil || !reflect.DeepEqual(events, []int32{3, 5}) {
		t.Fatalf("library reentry: %v, callback=%v, events=%v", err, cb.Err(), events)
	}
}

func TestNativeCloseDuringInitialization(t *testing.T) {
	inputs := []string{"object", "library"}
	if runtime.GOOS == "windows" {
		inputs = append(inputs, "import archive")
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			s := New(Options{})
			entered, release := make(chan struct{}), make(chan struct{})
			var events []int32
			name := "array_root"
			if input != "object" {
				name = "add"
			}
			cb, address := initializationCallback(t, func(args []abi.Value) (abi.Value, error) {
				event := int32(args[0].Bits)
				events = append(events, event)
				if event != 3 {
					return abi.Value{}, nil
				}
				err := s.WithInitializingAddress(name, func(p uintptr) error {
					close(entered)
					<-release
					if _, err := s.Resolve(name); !errors.Is(err, ErrClosed) {
						return fmt.Errorf("retired resolution: %v", err)
					}
					if err := initializationSum(s, name, 42); !errors.Is(err, ErrClosed) {
						return fmt.Errorf("retired scope: %v", err)
					}
					got, err := examplecall.Int32(p, 20, 22)
					if err == nil && got != 42 {
						err = fmt.Errorf("in-flight scoped call: %d", got)
					}
					return err
				})
				return abi.Value{}, err
			})
			defer s.Close()
			var path string
			if input == "object" {
				if err := s.Define("record_event", address); err != nil {
					t.Fatal(err)
				}
				path = compile(t, "testdata/lifecycle_arrays.c", filepath.Join(t.TempDir(), "arrays.o"))
				load(t, s, path)
			} else {
				load(t, s, compileSharedAdd(t, t.TempDir()))
				path = initializationLibrary(t, address)
				if input == "import archive" {
					path = filepath.Join(filepath.Dir(path), "initialization.a")
					load(t, s, path)
				}
			}
			var workers sync.WaitGroup
			var releaseOnce sync.Once
			closeReady, closeStart := make(chan struct{}), make(chan struct{})
			var closeOnce sync.Once
			defer func() {
				releaseOnce.Do(func() { close(release) })
				closeOnce.Do(func() { close(closeStart) })
				workers.Wait()
			}()
			initialized, closed := make(chan error, 1), make(chan error, 1)
			// llgo goroutines use host threads. Windows delays a new thread's
			// startup until DLL initialization releases the loader lock, so the
			// closing worker must be running before the constructor starts.
			workers.Add(1)
			go func() {
				defer workers.Done()
				close(closeReady)
				<-closeStart
				closed <- s.Close()
			}()
			<-closeReady
			workers.Add(1)
			go func() {
				defer workers.Done()
				if input == "library" {
					initialized <- s.Load(path)
				} else {
					root := "array_root"
					if input == "import archive" {
						root = "initialization_root"
					}
					initialized <- s.Link(root)
				}
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatalf("initializer did not enter scope: %v", cb.Err())
			}
			closeOnce.Do(func() { close(closeStart) })
			waitSessionRetired(t, s)
			select {
			case err := <-closed:
				t.Fatalf("Close freed active initialization: %v", err)
			default:
			}
			releaseOnce.Do(func() { close(release) })
			if err := <-initialized; !errors.Is(err, ErrClosed) {
				t.Fatalf("retired initialization: %v", err)
			}
			if err := <-closed; err != nil || cb.Err() != nil {
				t.Fatalf("initialization cleanup: %v, callback=%v", err, cb.Err())
			}
			want := []int32{3, 5}
			if input == "object" {
				want = []int32{3, 4, 5, 6}
				if runtime.GOOS == "linux" {
					want = append([]int32{0}, want...)
				}
			}
			if !reflect.DeepEqual(events, want) || len(s.libs) != 0 || s.image != nil || s.calls != 0 {
				t.Fatalf("retired resources/events: %v, want %v", events, want)
			}
		})
	}
}

func TestInitializingAddressContract(t *testing.T) {
	var absent *Session
	if err := absent.WithInitializingAddress("x", func(uintptr) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	s := New(Options{})
	defer s.Close()
	if err := s.WithInitializingAddress("x", func(uintptr) error { return nil }); !errors.Is(err, ErrNotInitializing) {
		t.Fatal(err)
	}
	s.initializing = true
	s.defined = map[string]uintptr{"x": 42}
	for _, name := range []string{"", "x\x00", "missing"} {
		if err := s.WithInitializingAddress(name, func(uintptr) error { t.Error("invalid scope invoked"); return nil }); err == nil {
			t.Fatalf("invalid scope %q accepted", name)
		}
	}
	if err := s.WithInitializingAddress("x", nil); err == nil {
		t.Fatal("nil scope accepted")
	}
	want := errors.New("adapter error")
	if err := s.WithInitializingAddress("x", func(p uintptr) error {
		if p != 42 {
			t.Fatalf("address: %d", p)
		}
		return s.WithInitializingAddress("x", func(uintptr) error { return want })
	}); err != want {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() != "adapter panic" {
				t.Error("scope changed panic")
			}
		}()
		_ = s.WithInitializingAddress("x", func(uintptr) error { panic("adapter panic") })
	}()
	if s.calls != 0 {
		t.Fatal("scope leaked guards")
	}
	s.initializing = false
	s.initErr = ErrInitialization
	if err := s.WithInitializingAddress("x", func(uintptr) error { return nil }); !errors.Is(err, ErrInitialization) {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.WithInitializingAddress("x", func(uintptr) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestNativeFailedInitializerRetainsActiveScope(t *testing.T) {
	s := New(Options{})
	entered, release := make(chan struct{}), make(chan struct{})
	used, closed := make(chan error, 1), make(chan error, 1)
	var workers sync.WaitGroup
	var releaseOnce sync.Once
	cb, address := initializationCallback(t, func(args []abi.Value) (abi.Value, error) {
		if int32(args[0].Bits) != 2 {
			return abi.Value{}, nil
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			used <- s.WithInitializingAddress("cinit_root", func(p uintptr) error {
				close(entered)
				<-release
				got, err := examplecall.Int32(p, 20, 22)
				if err == nil && got != 84 {
					err = fmt.Errorf("failed image scoped result: %d", got)
				}
				return err
			})
		}()
		select {
		case <-entered:
			return abi.Value{}, nil // XI fails while the independent scope is active.
		case <-time.After(5 * time.Second):
			return abi.Value{}, fmt.Errorf("independent initialization scope did not enter")
		}
	})
	defer s.Close()
	defer func() {
		releaseOnce.Do(func() { close(release) })
		workers.Wait()
	}()
	if err := s.Define("record_event", address); err != nil {
		t.Fatal(err)
	}
	obj := compile(t, "testdata/lifecycle_cinit.c", filepath.Join(t.TempDir(), "failed.o"), "-DFAIL_CODE=7")
	loadCInitializerFixture(t, s, obj)
	if err := s.Link("cinit_root"); !errors.Is(err, ErrInitialization) || cb.Err() != nil {
		t.Fatalf("failed initializer: %v, callback=%v", err, cb.Err())
	}
	retained := s.failedImage
	if retained == nil || s.image != nil || len(retained.mem) == 0 {
		t.Fatal("failed initialization lost its mapping")
	}
	workers.Add(1)
	go func() { defer workers.Done(); closed <- s.Close() }()
	waitSessionRetired(t, s)
	select {
	case err := <-closed:
		t.Fatalf("Close freed active failed-image scope: %v", err)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-used; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil || retained.mem != nil || s.failedImage != nil {
		t.Fatalf("failed image cleanup: %v", err)
	}
}

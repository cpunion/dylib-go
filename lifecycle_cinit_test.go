package dylib

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/cpunion/dylib-go/abi"
)

// On Windows, Load uses the production COFF parser. POSIX fixtures isolate the
// common C invocation and failed-session ownership contract with a private
// int(void) test table; they do not claim ELF/Mach-O int-array format support.
func loadCInitializerFixture(t *testing.T, s *Session, path string) {
	t.Helper()
	load(t, s, path)
	if runtime.GOOS == "windows" {
		return
	}
	o := s.files[len(s.files)-1].obj
	for _, section := range o.sections {
		if section != nil && (section.name == ".dylib_c_init" || section.name == "__c_init") {
			section.lifecycle = lifecycleCInit
			if err := configureLifecycle(section, o.info.Bits); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("C initializer fixture table missing")
}

func TestCOFFCInitializerMetadata(t *testing.T) {
	for _, triple := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(triple, func(t *testing.T) {
			path := compile(t, "testdata/lifecycle_cinit.c", filepath.Join(t.TempDir(), "cinit.o"), "--target="+triple)
			b, err := readFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(path, b)
			if err != nil || len(f.info.Unsupported) != 0 {
				t.Fatalf("COFF C initializer metadata: %v, %v", f, err)
			}
			counts := make(map[lifecycleKind]uint64)
			for _, s := range f.obj.sections {
				if s != nil {
					counts[s.lifecycle] += s.size / uint64(f.info.Bits/8)
				}
			}
			if counts[lifecycleCInit] != 6 || counts[lifecycleInit] != 1 || counts[lifecycleFini] != 2 {
				t.Fatalf("COFF table kinds: %v", counts)
			}
		})
	}
}

func TestNativeCInitializerSuccess(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	obj := compile(t, "testdata/lifecycle_cinit.c", filepath.Join(dir, "cinit.o"))
	host, _, read := lifecycleObserver(t, dir)
	s := New(Options{})
	defer s.Close()
	load(t, s, host)
	loadCInitializerFixture(t, s, obj)
	expectEvents(t, read)
	call(t, s, "cinit_root", 0, 0, 42)
	expectEvents(t, read, 1, 2, 3, 4)
	if err := s.Link("cinit_root"); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 1, 2, 3, 4)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 1, 2, 3, 4, 7, 8, 5, 6)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 1, 2, 3, 4, 7, 8, 5, 6)
}

func TestNativeCInitializerFailure(t *testing.T) {
	needNative(t)
	for _, code := range []int32{7, -7} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			dir := t.TempDir()
			obj := compile(t, "testdata/lifecycle_cinit.c", filepath.Join(dir, "failed.o"), fmt.Sprintf("-DFAIL_CODE=%d", code))
			host, _, read := lifecycleObserver(t, dir)
			s := New(Options{})
			defer s.Close()
			load(t, s, host)
			loadCInitializerFixture(t, s, obj)
			err := s.Link("cinit_root")
			var initErr *InitializationError
			if !errors.Is(err, ErrInitialization) || !errors.As(err, &initErr) || initErr.Code != code || initErr.Object != obj {
				t.Fatalf("initializer result: %v", err)
			}
			wantOffset := uint64(unsafe.Sizeof(uintptr(0)))
			wantSection := ".CRT$XIC"
			if runtime.GOOS != "windows" {
				wantOffset *= 2
				wantSection = ".dylib_c_init"
				if runtime.GOOS == "darwin" {
					wantSection = "__c_init"
				}
			}
			if initErr.Section != wantSection || initErr.Offset != wantOffset || s.image != nil || s.failedImage == nil || s.failedImage.initialized || len(s.failedImage.mem) == 0 {
				t.Fatalf("failure ownership/entry: error=%+v published=%v retained=%v", initErr, s.image != nil, s.failedImage != nil)
			}
			expectEvents(t, read, 1, 2)
			// Every operation rejects the same permanent failure without rerunning
			// initializers, preparing a CIF, loading a DLL, or publishing a symbol.
			for _, operation := range []func() error{
				func() error { return s.Link("cinit_root") },
				func() error { _, e := s.Lookup("cinit_root"); return e },
				func() error { _, e := s.Resolve("cinit_root"); return e },
				func() error {
					_, e := s.Bind("cinit_root", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}})
					return e
				},
				func() error { return s.Load(obj) },
				func() error { return s.Define("another", 1) },
			} {
				if e := operation(); e != err {
					t.Fatalf("failed session changed result: %v, want %v", e, err)
				}
			}
			expectEvents(t, read, 1, 2)
			retained := s.failedImage
			if e := s.Close(); e != nil {
				t.Fatal(e)
			}
			if retained.mem != nil || s.failedImage != nil {
				t.Fatal("failed mapping retained after Close")
			}
			expectEvents(t, read, 1, 2, 7, 8) // No static terminators for incomplete initialization.
			if _, e := s.Lookup("cinit_root"); !errors.Is(e, ErrClosed) {
				t.Fatalf("closed failed session: %v", e)
			}
			if e := s.Close(); e != nil {
				t.Fatal(e)
			}
			expectEvents(t, read, 1, 2, 7, 8)
		})
	}
}

func TestNativeCInitializerValidationAndRetry(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	obj := compile(t, "testdata/lifecycle_cinit.c", filepath.Join(dir, "cinit.o"))
	host, _, read := lifecycleObserver(t, dir)
	s := New(Options{})
	defer s.Close()
	loadCInitializerFixture(t, s, obj)
	if err := s.Link("cinit_root"); err == nil || !strings.Contains(err.Error(), "record_event") || s.initErr != nil || s.failedImage != nil {
		t.Fatalf("dependency validation became permanent: %v", err)
	}
	load(t, s, host)
	if err := s.Link("absent_root"); err == nil || s.initErr != nil {
		t.Fatalf("root validation became permanent: %v", err)
	}
	expectEvents(t, read)
	call(t, s, "cinit_root", 0, 0, 42)
	expectEvents(t, read, 1, 2, 3, 4)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 1, 2, 3, 4, 7, 8, 5, 6)

	bad := compile(t, "testdata/lifecycle_cinit.c", filepath.Join(dir, "bad.o"), "-DBAD_CINIT_POINTER")
	host, _, read = lifecycleObserver(t, t.TempDir())
	s = New(Options{})
	defer s.Close()
	load(t, s, host)
	loadCInitializerFixture(t, s, bad)
	if err := s.Link("cinit_root"); err == nil || !strings.Contains(err.Error(), "outside executable") || s.initErr != nil || s.failedImage != nil || s.image != nil {
		t.Fatalf("invalid entry executed or became permanent: %v", err)
	}
	expectEvents(t, read)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read)
}

func TestNativeFailedCInitializerCleanupReentersGo(t *testing.T) {
	needNative(t)
	if !abi.Available() {
		t.Skip("requires -tags libffi for the test callback")
	}
	s := New(Options{})
	var events []int32
	callback, err := abi.NewCallback(abi.Signature{Args: []abi.Type{abi.I32}}, func(args []abi.Value) (abi.Value, error) {
		event := int32(args[0].Bits)
		events = append(events, event)
		if event == 7 || event == 8 {
			if _, err := s.Lookup("cinit_root"); !errors.Is(err, ErrClosed) {
				return abi.Value{}, fmt.Errorf("cleanup lookup: %v", err)
			}
		}
		return abi.Value{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	lease, err := callback.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	defer s.Close() // Cleanup while the external callback lease is still alive.
	address, err := lease.Address()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Define("record_event", address); err != nil {
		t.Fatal(err)
	}
	obj := compile(t, "testdata/lifecycle_cinit.c", filepath.Join(t.TempDir(), "failed.o"), "-DFAIL_CODE=7")
	loadCInitializerFixture(t, s, obj)
	if symbol, err := s.Resolve("cinit_root"); !errors.Is(err, ErrInitialization) || symbol != nil {
		t.Fatalf("initializer failure published a symbol: %v, %v", symbol, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if callback.Err() != nil || !reflect.DeepEqual(events, []int32{1, 2, 7, 8}) {
		t.Fatalf("cleanup callbacks: events=%v, error=%v", events, callback.Err())
	}
}

func TestNativeCInitializerFailureFromBind(t *testing.T) {
	needNative(t)
	if !abi.Available() {
		t.Skip("requires -tags libffi for the prepared binding")
	}
	dir := t.TempDir()
	obj := compile(t, "testdata/lifecycle_cinit.c", filepath.Join(dir, "failed.o"), "-DFAIL_CODE=7")
	host, _, read := lifecycleObserver(t, dir)
	s := New(Options{})
	defer s.Close()
	load(t, s, host)
	loadCInitializerFixture(t, s, obj)
	function, err := s.Bind("cinit_root", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}})
	if !errors.Is(err, ErrInitialization) || function != nil || len(s.plans) != 0 || s.image != nil {
		t.Fatalf("failed binding published a plan/function: %v, %v", function, err)
	}
	expectEvents(t, read, 1, 2)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 1, 2, 7, 8)
}

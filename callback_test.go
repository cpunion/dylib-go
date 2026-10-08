package dylib

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/abi/signature"
)

func callbackLibrary(t *testing.T, input string) *Session {
	t.Helper()
	return nativeABILibrary(t, "testdata/callbacks.c", input)
}

// All dynamic ABI fixtures run as standalone objects, archives, and OS libraries.
func nativeABILibrary(t *testing.T, source, input string) *Session {
	t.Helper()
	if !abi.Available() {
		t.Skip("requires libffi")
	}
	needNative(t)
	dir := t.TempDir()
	var path string
	switch input {
	case "object", "archive":
		path = compile(t, source, filepath.Join(dir, "native.o"))
		if input == "archive" {
			archive := filepath.Join(dir, "native.a")
			command(t, "ar", "rcs", archive, path)
			path = archive
		}
	case "library":
		name, flag := "native.so", "-shared"
		if runtime.GOOS == "darwin" {
			name, flag = "native.dylib", "-dynamiclib"
		}
		if runtime.GOOS == "windows" {
			name = "native.dll"
		}
		path = filepath.Join(dir, name)
		args := []string{flag, "-O0", source, "-o", path}
		if runtime.GOOS != "windows" {
			args = append(args, "-fPIC")
		}
		if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
			args = append(args, "--target=x86_64-apple-macosx11")
		}
		command(t, compiler(), args...)
	}
	// Aggregate copies/initializers may call the host's memcpy or memset.
	s := New(Options{ProcessSymbols: runtime.GOOS != "windows"})
	if runtime.GOOS == "windows" && input != "library" {
		root := os.Getenv("SystemRoot")
		crt := filepath.Join(root, "System32", "msvcrt.dll")
		if runtime.GOARCH == "386" {
			wow := filepath.Join(root, "SysWOW64", "msvcrt.dll")
			if _, err := os.Stat(wow); err == nil {
				crt = wow
			}
		}
		load(t, s, crt)
	}
	load(t, s, path)
	t.Cleanup(func() { s.Close() })
	return s
}

func callbackSignature(t *testing.T, text string) abi.Signature {
	t.Helper()
	decl, err := signature.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	return decl.Signature
}

func callbackCall(t *testing.T, s *Session, decl string, cb *abi.Callback, args ...abi.Value) abi.Value {
	t.Helper()
	parsed, err := signature.Parse(decl)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Bind(parsed.Name, parsed.Signature)
	if err != nil {
		t.Fatal(err)
	}
	var result abi.Value
	err = cb.WithAddress(func(address uintptr) error {
		var err error
		values := append([]abi.Value{abi.Ptr(address)}, args...)
		result, err = f.Call(values...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestNativeCallbacksScalars(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			s := callbackLibrary(t, input)
			zero, err := abi.NewCallback(callbackSignature(t, "func cb()int32"), func(args []abi.Value) (abi.Value, error) {
				if len(args) != 0 {
					return abi.Value{}, errors.New("unexpected zero-argument callback input")
				}
				return abi.Int32(42), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer zero.Close()
			if v := callbackCall(t, s, "func invoke_zero(unsafe.Pointer)int32", zero); v != abi.Int32(42) || zero.Err() != nil {
				t.Fatalf("zero arguments: %+v %v", v, zero.Err())
			}
			for _, tc := range []struct {
				callback, call string
				args           []abi.Value
				want           abi.Value
			}{
				{"func cb(int8)int8", "func invoke_narrow(unsafe.Pointer,int32)int32", []abi.Value{abi.Int32(-128)}, abi.Int32(-128)},
				{"func cb(uint8)uint8", "func invoke_u8(unsafe.Pointer,uint32)uint32", []abi.Value{abi.Uint32(255)}, abi.Uint32(255)},
				{"func cb(int16)int16", "func invoke_i16(unsafe.Pointer,int32)int32", []abi.Value{abi.Int32(-32768)}, abi.Int32(-32768)},
				{"func cb(uint16)uint16", "func invoke_u16(unsafe.Pointer,uint32)uint32", []abi.Value{abi.Uint32(65535)}, abi.Uint32(65535)},
				{"func cb(int64)int64", "func invoke_i64(unsafe.Pointer,int64)int64", []abi.Value{abi.Int64(math.MinInt64)}, abi.Int64(math.MinInt64)},
				{"func cb(uint64)uint64", "func invoke_u64(unsafe.Pointer,uint64)uint64", []abi.Value{abi.Uint64(math.MaxUint64)}, abi.Uint64(math.MaxUint64)},
				{"func cb(bool)bool", "func invoke_bool(unsafe.Pointer,uint32)uint32", []abi.Value{abi.Uint32(1)}, abi.Uint32(1)},
				{"func cb(float32)float32", "func invoke_float(unsafe.Pointer,float32)float32", []abi.Value{abi.Float32(20.5)}, abi.Float32(20.5)},
				{"func cb(unsafe.Pointer)unsafe.Pointer", "func invoke_pointer(unsafe.Pointer,unsafe.Pointer)unsafe.Pointer", []abi.Value{abi.Ptr(0x1234)}, abi.Ptr(0x1234)},
			} {
				t.Run(tc.callback, func(t *testing.T) {
					cb, err := abi.NewCallback(callbackSignature(t, tc.callback), func(args []abi.Value) (abi.Value, error) { return args[0], nil })
					if err != nil {
						t.Fatal(err)
					}
					defer cb.Close()
					v := callbackCall(t, s, tc.call, cb, tc.args...)
					if v != tc.want || cb.Err() != nil {
						t.Fatalf("result=%+v want=%+v callback=%v", v, tc.want, cb.Err())
					}
				})
			}
			cb, err := abi.NewCallback(callbackSignature(t, "func cb(int32,float64,float32,uint64)float64"), func(args []abi.Value) (abi.Value, error) {
				v := float64(int32(args[0].Bits)) + math.Float64frombits(args[1].Bits) + float64(math.Float32frombits(uint32(args[2].Bits))) + float64(args[3].Bits)
				return abi.Float64(v), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer cb.Close()
			if v := callbackCall(t, s, "func invoke_mixed(unsafe.Pointer)float64", cb); v != abi.Float64(42) {
				t.Fatal(v)
			}
			var captured int32
			void, err := abi.NewCallback(callbackSignature(t, "func cb(int32)"), func(args []abi.Value) (abi.Value, error) { captured = int32(args[0].Bits); return abi.Value{}, nil })
			if err != nil {
				t.Fatal(err)
			}
			defer void.Close()
			if v := callbackCall(t, s, "func invoke_void(unsafe.Pointer,int32)int32", void, abi.Int32(42)); v != abi.Int32(42) || captured != 42 {
				t.Fatalf("void callback: %+v %d", v, captured)
			}
		})
	}
}

func TestNativeCallbacksRecords(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			s := callbackLibrary(t, input)
			for _, tc := range []struct{ name, typ, value string }{
				{"invoke_pair", "struct{a,b int32}", "{20,22}"},
				{"invoke_nested", "struct{tag int8;p struct{a,b int32};extra float64}", "{1,{20,20},1}"},
				{"invoke_big", "struct{a,b,c int64}", "{20,21,1}"},
				{"invoke_floats", "struct{x,y float32}", "{20.5,21.5}"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					decl := callbackSignature(t, "func cb("+tc.typ+")"+tc.typ)
					cb, err := abi.NewCallback(decl, func(args []abi.Value) (abi.Value, error) { return args[0], nil })
					if err != nil {
						t.Fatal(err)
					}
					defer cb.Close()
					value, err := signature.ParseTypedValue(decl.ArgumentType(0), tc.value)
					if err != nil {
						t.Fatal(err)
					}
					got := callbackCall(t, s, "func "+tc.name+"(unsafe.Pointer,"+tc.typ+")"+tc.typ, cb, value)
					if !reflect.DeepEqual(got, value) {
						t.Fatalf("got=%+v want=%+v", got.Aggregate, value.Aggregate)
					}
					if err := cb.Err(); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestNativeCallbackFailures(t *testing.T) {
	s := callbackLibrary(t, "object")
	failure := errors.New("handler rejected input")
	for _, handler := range []abi.CallbackFunc{
		func([]abi.Value) (abi.Value, error) { return abi.Value{}, failure },
		func([]abi.Value) (abi.Value, error) { panic("handler panic") },
		func([]abi.Value) (abi.Value, error) { return abi.Float64(42), nil },
		func([]abi.Value) (abi.Value, error) { return abi.AddressOf(new(abi.Value)), nil },
	} {
		cb, err := abi.NewCallback(callbackSignature(t, "func cb(int32,int32)int32"), handler)
		if err != nil {
			t.Fatal(err)
		}
		v := callbackCall(t, s, "func invoke_i32(unsafe.Pointer,int32,int32)int32", cb, abi.Int32(20), abi.Int32(22))
		if v != abi.Int32(0) || cb.Err() == nil {
			t.Fatalf("failure result=%+v err=%v", v, cb.Err())
		}
		cb.Close()
	}
	// Pointees also fail when the physical result type is a pointer.
	cb, err := abi.NewCallback(callbackSignature(t, "func cb(unsafe.Pointer)unsafe.Pointer"), func([]abi.Value) (abi.Value, error) { v := abi.Int32(42); return abi.AddressOf(&v), nil })
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()
	if v := callbackCall(t, s, "func invoke_pointer(unsafe.Pointer,unsafe.Pointer)unsafe.Pointer", cb, abi.Ptr(0)); v != abi.Ptr(0) || cb.Err() == nil {
		t.Fatalf("escaping pointer: %+v %v", v, cb.Err())
	}
}

func TestNativeCallbackLeasesAndRetirement(t *testing.T) {
	s := callbackLibrary(t, "object")
	sig := callbackSignature(t, "func cb(int32,int32)int32")
	var mu sync.Mutex
	captured := int32(0)
	cb, err := abi.NewCallback(sig, func(args []abi.Value) (abi.Value, error) {
		mu.Lock()
		captured++
		mu.Unlock()
		return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()
	sig.Args[0] = abi.Void // Callback owns its signature snapshot.
	lease, err := cb.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	address, err := lease.Address()
	if err != nil {
		t.Fatal(err)
	}
	store, err := s.Bind("store_callback", callbackSignature(t, "func store_callback(unsafe.Pointer)"))
	if err != nil {
		t.Fatal(err)
	}
	invoke, err := s.Bind("call_stored", callbackSignature(t, "func call_stored(int32,int32)int32"))
	if err != nil {
		t.Fatal(err)
	}
	clear, err := s.Bind("clear_callback", callbackSignature(t, "func clear_callback()"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Call(abi.Ptr(address)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				v, err := invoke.Call(abi.Int32(20), abi.Int32(22))
				if err != nil || v != abi.Int32(42) {
					t.Errorf("retained callback: %+v %v", v, err)
				}
			}
		}()
	}
	wg.Wait()
	if captured != 40 {
		t.Fatal(captured)
	}
	closed := make(chan struct{})
	go func() { cb.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("closed while native registration lease is alive")
	case <-time.After(20 * time.Millisecond):
	}
	if _, err := clear.Call(); err != nil {
		t.Fatal(err)
	}
	lease.Close()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("close did not finish after lease release")
	}
	if _, err := cb.Acquire(); !errors.Is(err, abi.ErrCallbackClosed) {
		t.Fatal(err)
	}
	if _, err := lease.Address(); !errors.Is(err, abi.ErrCallbackClosed) {
		t.Fatal(err)
	}
	lease.Close()
	cb.Close()
}

func TestNativeCallbackErrorHistoryAndZeroRecord(t *testing.T) {
	s := callbackLibrary(t, "object")
	failure := errors.New("first callback failure")
	calls := 0
	sig := callbackSignature(t, "func cb(struct{a,b,c int64})struct{a,b,c int64}")
	cb, err := abi.NewCallback(sig, func(args []abi.Value) (abi.Value, error) {
		calls++
		switch calls {
		case 1:
			return abi.Value{}, failure
		case 2:
			return args[0], nil
		default:
			panic("later callback failure")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()
	value, err := signature.ParseTypedValue(sig.ArgumentType(0), "{20,21,1}")
	if err != nil {
		t.Fatal(err)
	}
	zero, err := signature.ParseTypedValue(sig.ReturnType(), "{0,0,0}")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []abi.Value{zero, value, zero} {
		got := callbackCall(t, s, "func invoke_big(unsafe.Pointer,struct{a,b,c int64})struct{a,b,c int64}", cb, value)
		if !reflect.DeepEqual(got, want) || !errors.Is(cb.Err(), failure) {
			t.Fatalf("result=%+v want=%+v err=%v", got.Aggregate, want.Aggregate, cb.Err())
		}
	}
	cb.Close()
	if !errors.Is(cb.Err(), failure) {
		t.Fatal("close lost the callback failure", cb.Err())
	}
}

func TestNativeCallbackWindows386Conventions(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "386" {
		t.Skip("Windows i386 conventions")
	}
	s := callbackLibrary(t, "object")
	for _, tc := range []struct {
		name       string
		convention abi.Convention
	}{{"invoke_stdcall", abi.StdCall}, {"invoke_fastcall", abi.FastCall}} {
		sig := callbackSignature(t, "func cb(int32,int32)int32")
		sig.Convention = tc.convention
		cb, err := abi.NewCallback(sig, func(args []abi.Value) (abi.Value, error) {
			return abi.Int32(int32(args[0].Bits) + int32(args[1].Bits)), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 50; i++ {
			if got := callbackCall(t, s, "func "+tc.name+"(unsafe.Pointer,int32,int32)int32", cb, abi.Int32(20), abi.Int32(22)); got != abi.Int32(42) {
				t.Fatal(got)
			}
		}
		cb.Close()
	}
}

func callbackThreadLibrary(t *testing.T, source string) *Session {
	t.Helper()
	if !abi.Available() {
		t.Skip("callbacks require libffi")
	}
	needShared(t)
	dir := t.TempDir()
	name, flag := "threads.so", "-shared"
	if runtime.GOOS == "darwin" {
		name, flag = "threads.dylib", "-dynamiclib"
	}
	if runtime.GOOS == "windows" {
		name = "threads.dll"
	}
	path := filepath.Join(dir, name)
	args := []string{flag, "-O0", source, "-o", path}
	if runtime.GOOS != "windows" {
		args = append(args, "-fPIC", "-pthread")
	}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		args = append(args, "--target=x86_64-apple-macosx11")
	}
	command(t, compiler(), args...)
	s := New(Options{})
	load(t, s, path)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNativeCallbacksOnForeignThreads(t *testing.T) {
	s := callbackThreadLibrary(t, "testdata/callback_threads.c")
	var mu sync.Mutex
	calls := 0
	capture := []int32{20, 22}
	cb, err := abi.NewCallback(callbackSignature(t, "func cb(int32,int32)int32"), func(values []abi.Value) (abi.Value, error) {
		// Enter from OS-created C threads, allocate Go memory, and collect
		// while other native threads may be inside the same Go handler.
		copy := append([]int32(nil), capture...)
		runtime.GC()
		mu.Lock()
		calls++
		mu.Unlock()
		return abi.Int32(copy[0] + copy[1] + int32(values[0].Bits) - 20), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()
	runtime.GC() // Captures must remain rooted by the native handle owner.
	if v := callbackCall(t, s, "func invoke_threads(unsafe.Pointer,int32)int32", cb, abi.Int32(4)); v != abi.Int32(1680) {
		t.Fatalf("threaded result: %+v", v)
	}
	mu.Lock()
	actualCalls := calls
	mu.Unlock()
	if actualCalls != 40 || cb.Err() != nil {
		t.Fatalf("calls=%d error=%v", actualCalls, cb.Err())
	}
}

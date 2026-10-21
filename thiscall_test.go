package dylib

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func thiscallSession(t *testing.T, target, opt, input string) *Session {
	t.Helper()
	dir := t.TempDir()
	object := compile(t, "testdata/thiscall.c", filepath.Join(dir, "thiscall.o"), "--target="+target, opt)
	path := object
	switch input {
	case "archive":
		path = filepath.Join(dir, "thiscall.a")
		command(t, "ar", "rcs", path, object)
	case "library":
		path = filepath.Join(dir, "thiscall.dll")
		flags := []string{"--target=" + target, opt, "-shared", object, "-o", path}
		if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
			flags = append(flags, "--sysroot="+root, "--rtlib=libgcc")
		}
		command(t, compiler(), flags...)
	}
	s := New(Options{})
	load(t, s, filepath.Join(os.Getenv("SystemRoot"), "SysWOW64", "msvcrt.dll"), path)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func TestNativeWindows386Thiscall(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "386" {
		t.Skip("Windows 386 thiscall")
	}
	if !abi.Available() {
		t.Skip("requires libffi")
	}
	needNative(t)
	for _, target := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run(target, func(t *testing.T) {
			for _, opt := range []string{"-O0", "-O2"} {
				t.Run(opt, func(t *testing.T) {
					kinds := []string{"object", "archive"}
					if target == "i686-w64-windows-gnu" {
						kinds = append(kinds, "library")
					}
					for _, kind := range kinds {
						t.Run(kind, func(t *testing.T) {
							s := thiscallSession(t, target, opt, kind)
							exerciseThiscall(t, s)
						})
					}
				})
			}
		})
	}
}

func exerciseThiscall(t *testing.T, s *Session) {
	t.Helper()
	bind := func(name, decl string, conv abi.Convention) *Function {
		t.Helper()
		sig := callbackSignature(t, decl)
		sig.Convention = conv
		f, err := s.Bind(name, sig)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	invoke := func(f *Function, args ...abi.Value) abi.Value {
		t.Helper()
		v, err := f.Call(args...)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	receiver := invoke(bind("this_state", "func f()unsafe.Pointer", abi.CDecl))
	if receiver.Bits == 0 {
		t.Fatal("null receiver")
	}
	add := bind("this_add", "func f(unsafe.Pointer,int32)int32", abi.ThisCall)
	mixedDecl := "func f(unsafe.Pointer,int8,uint16,float32,float64,uint64,int32)float64"
	mixed := bind("this_mixed", mixedDecl, abi.ThisCall)
	tail := []abi.Value{abi.Int8(-120), abi.Uint16(65000), abi.Float32(1.25), abi.Float64(-2.25), abi.Uint64(1<<40 + 5), abi.Int32(-20)}
	want := abi.Float64(20 - 120 + 65000 + 1.25 - 2.25 + (1<<40 + 5) - 20)
	wide := bind("this_wide", "func f(unsafe.Pointer,uint64)uint64", abi.ThisCall)
	pointer := bind("this_pointer", "func f(unsafe.Pointer,unsafe.Pointer)unsafe.Pointer", abi.ThisCall)
	void := bind("this_void", "func f(unsafe.Pointer,int32)", abi.ThisCall)
	payloadSig := callbackSignature(t, "func f(unsafe.Pointer,struct{a int32;b float64;c int32},int32)int32")
	payloadSig.Convention = abi.ThisCall
	payload, err := s.Bind("this_payload", payloadSig)
	if err != nil {
		t.Fatal(err)
	}
	record, err := abi.StructValue(payloadSig.ArgumentType(1), abi.Int32(3), abi.Float64(4), abi.Int32(5))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("scalars_and_receiver", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			if got := invoke(add, receiver, abi.Int32(22)); got != abi.Int32(42) {
				t.Fatal(got)
			}
			if got := invoke(mixed, append([]abi.Value{receiver}, tail...)...); got != want {
				t.Fatal("mixed:", got, want)
			}
			if got := invoke(wide, receiver, abi.Uint64(math.MaxUint64-7)); got != abi.Uint64((math.MaxUint64-7)^uint64(20)<<48) {
				t.Fatal("wide:", got)
			}
			if got := invoke(pointer, receiver, receiver); got != receiver {
				t.Fatal("pointer:", got)
			}
			if got := invoke(void, receiver, abi.Int32(20)); got.Type != abi.Void {
				t.Fatal("void:", got)
			}
			if got := invoke(payload, receiver, record, abi.Int32(10)); got != abi.Int32(42) {
				t.Fatal("record argument:", got)
			}
		}
		if got := invoke(bind("this_calls", "func f(unsafe.Pointer)int32", abi.CDecl), receiver); got != abi.Int32(600) {
			t.Fatal("receiver state:", got)
		}
	})
	t.Run("temporary_receiver", func(t *testing.T) {
		sig := callbackSignature(t, "func f(*struct{bias,calls int32},int32)int32")
		sig.Convention = abi.ThisCall
		f, err := s.Bind("this_add", sig)
		if err != nil {
			t.Fatal(err)
		}
		state, err := abi.StructValue(*sig.ArgumentType(0).Elem, abi.Int32(20), abi.Int32(0))
		if err != nil {
			t.Fatal(err)
		}
		if got := invoke(f, abi.AddressOf(&state), abi.Int32(22)); got != abi.Int32(42) || state.Aggregate.Fields[1] != abi.Int32(1) {
			t.Fatal(got, state)
		}
	})
	t.Run("native_function_pointer", func(t *testing.T) {
		entry := invoke(bind("this_factory", "func f()unsafe.Pointer", abi.CDecl))
		sig := callbackSignature(t, mixedDecl)
		sig.Convention = abi.ThisCall
		plan, err := abi.Prepare(sig)
		if err != nil {
			t.Fatal(err)
		}
		defer plan.Close()
		symbol, err := s.Resolve("this_mixed")
		if err != nil {
			t.Fatal(err)
		}
		if err := symbol.WithAddress(func(address uintptr) error {
			if address != uintptr(entry.Bits) {
				t.Fatal("factory identity:", address, entry)
			}
			got, err := plan.Call(uintptr(entry.Bits), append([]abi.Value{receiver}, tail...)...)
			if err != nil || got != want {
				t.Fatal(got, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("zero_stack_callback", func(t *testing.T) {
		calls := 0
		cb, err := abi.NewCallback(abi.Signature{Convention: abi.ThisCall, Args: []abi.Type{abi.Pointer}}, func(args []abi.Value) (abi.Value, error) {
			calls++
			if len(args) != 1 || args[0] != receiver {
				return abi.Value{}, errors.New("wrong logical receiver arguments")
			}
			return abi.Value{}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer cb.Close()
		apply := bind("this_notify_apply", "func f(unsafe.Pointer,unsafe.Pointer)", abi.CDecl)
		if err := cb.WithAddress(func(address uintptr) error {
			for i := 0; i < 100; i++ {
				invoke(apply, abi.Ptr(address), receiver)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if calls != 100 || cb.Err() != nil {
			t.Fatal(calls, cb.Err())
		}
	})
	t.Run("scalar_callback", func(t *testing.T) {
		sig := callbackSignature(t, "func f(unsafe.Pointer,int32)int32")
		sig.Convention = abi.ThisCall
		cb, err := abi.NewCallback(sig, func(args []abi.Value) (abi.Value, error) { return add.Call(args...) })
		if err != nil {
			t.Fatal(err)
		}
		defer cb.Close()
		apply := bind("this_int_apply", "func f(unsafe.Pointer,unsafe.Pointer,int32)int32", abi.CDecl)
		if err := cb.WithAddress(func(address uintptr) error {
			for i := 0; i < 100; i++ {
				if got := invoke(apply, abi.Ptr(address), receiver, abi.Int32(22)); got != abi.Int32(42) {
					t.Fatal(got)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if cb.Err() != nil {
			t.Fatal(cb.Err())
		}
	})

	t.Run("record_callback", func(t *testing.T) {
		cb, err := abi.NewCallback(payloadSig, func(args []abi.Value) (abi.Value, error) { return payload.Call(args...) })
		if err != nil {
			t.Fatal(err)
		}
		defer cb.Close()
		apply := bind("this_payload_apply", "func f(unsafe.Pointer,unsafe.Pointer,struct{a int32;b float64;c int32},int32)int32", abi.CDecl)
		err = cb.WithAddress(func(address uintptr) error {
			for i := 0; i < 50; i++ {
				if got := invoke(apply, abi.Ptr(address), receiver, record, abi.Int32(10)); got != abi.Int32(42) {
					t.Fatal("record callback:", got)
				}
			}
			return nil
		})
		if err != nil || cb.Err() != nil {
			t.Fatal(err, cb.Err())
		}
	})

	t.Run("nested_callback", func(t *testing.T) {
		calls := 0
		sig := callbackSignature(t, mixedDecl)
		sig.Convention = abi.ThisCall
		cb, err := abi.NewCallback(sig, func(args []abi.Value) (abi.Value, error) {
			calls++
			if len(args) != 7 || args[0] != receiver {
				t.Error("callback receiver:", args)
				return abi.Float64(0), errors.New("wrong receiver")
			}
			for i, v := range tail {
				if args[i+1] != v {
					t.Errorf("callback argument %d: %v", i, args[i+1])
				}
			}
			// Go -> thiscall C -> thiscall Go -> thiscall C on the same receiver.
			got, err := add.Call(args[0], abi.Int32(22))
			if err != nil {
				return abi.Value{}, err
			}
			if got != abi.Int32(42) {
				return abi.Value{}, errors.New("nested thiscall mismatch")
			}
			return mixed.Call(args...)
		})
		if err != nil {
			t.Fatal(err)
		}
		defer cb.Close()
		lease, err := cb.Acquire()
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()
		address, err := lease.Address()
		if err != nil {
			t.Fatal(err)
		}
		apply := bind("this_apply", "func f(unsafe.Pointer,unsafe.Pointer,int8,uint16,float32,float64,uint64,int32)float64", abi.CDecl)
		forward := bind("this_forward", "func f(unsafe.Pointer,unsafe.Pointer,int8,uint16,float32,float64,uint64,int32)float64", abi.ThisCall)
		for i := 0; i < 100; i++ {
			if got := invoke(apply, append([]abi.Value{abi.Ptr(address), receiver}, tail...)...); got != want {
				t.Fatal("cdecl callback:", got, want)
			}
			if got := invoke(forward, append([]abi.Value{receiver, abi.Ptr(address)}, tail...)...); got != want {
				t.Fatal("thiscall callback:", got, want)
			}
		}
		if calls != 200 || cb.Err() != nil {
			t.Fatal("callback count/error:", calls, cb.Err())
		}
	})
}

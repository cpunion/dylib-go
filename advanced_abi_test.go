package dylib

import (
	"errors"
	"math"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/abi/signature"
)

func TestVariadicABIAndPreparedLifetime(t *testing.T) {
	if !abi.Available() {
		t.Skip("requires libffi")
	}
	needShared(t)
	dir := t.TempDir()
	library := filepath.Join(dir, "varargs.so")
	switch runtime.GOOS {
	case "darwin":
		library = filepath.Join(dir, "varargs.dylib")
		command(t, compiler(), "-dynamiclib", "testdata/variadic.c", "-o", library)
	case "windows":
		library = filepath.Join(dir, "varargs.dll")
		command(t, compiler(), "-shared", "testdata/variadic.c", "-Wl,--export-all-symbols", "-o", library)
	default:
		command(t, compiler(), "-shared", "-fPIC", "testdata/variadic.c", "-o", library)
	}
	s := New(Options{})
	defer s.Close()
	load(t, s, library)
	for _, tc := range []struct {
		text     string
		fixed    int
		expected float64
	}{
		{"var_promotions(-65500:int32,-5:int8,65535:uint16,true:bool,10.5:float32,0.5:float64)float64", 1, 42},
		{"var_fixed(20.5:float32,1:int32,21.5:float32)float64", 2, 42},
		{"var_fixed(20:float32,3:int32,10:float32,5.5:float64,6.5:float32)float64", 2, 42},
		{"var_empty(42:int32)int32", 1, 42},
		{"var_pair(2:int32,{a:20,b:20}:struct{a,b int32})int32", 1, 42},
		{"var_pointer(0:int32,&40:*int32)int32", 1, 42},
	} {
		t.Run(tc.text, func(t *testing.T) {
			input, err := signature.ParseCall(tc.text)
			if err != nil {
				t.Fatal(err)
			}
			input.Signature.Variadic, input.Signature.FixedArgs = true, tc.fixed
			f, err := s.Bind(input.Name, input.Signature)
			if err != nil {
				t.Fatal(err)
			}
			// Binding snapshots logical and physical types; caller edits cannot alter the CIF.
			input.Signature.Args[0] = abi.Void
			for i := 0; i < 10; i++ {
				if input.Name == "var_pointer" {
					*input.Args[1].Pointee = abi.Int32(40)
				}
				v, err := f.Call(input.Args...)
				actual := float64(int32(v.Bits))
				if v.Type == abi.F64 {
					actual = math.Float64frombits(v.Bits)
				}
				if err != nil || actual != tc.expected {
					t.Fatalf("got %+v (%g), %v", v, actual, err)
				}
			}
			if input.Name == "var_pointer" && *input.Args[1].Pointee != abi.Int32(42) {
				t.Fatal("variadic pointee mutation lost")
			}
			if _, err := f.Call(); err == nil {
				t.Fatal("wrong call shape accepted")
			}
		})
	}
	entry, err := s.Resolve("var_empty")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := abi.Prepare(abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}, Variadic: true, FixedArgs: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	if err := entry.WithAddress(func(address uintptr) error {
		v, err := plan.Call(address, abi.Int32(42))
		if err == nil && v != abi.Int32(42) {
			t.Fatal(v)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	plan.Close()
	if _, err := plan.Call(1, abi.Int32(42)); !errors.Is(err, abi.ErrClosed) {
		t.Fatalf("closed plan: %v", err)
	}
	bound, err := s.Bind("var_empty", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}, Variadic: true, FixedArgs: 1})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if len(s.plans) != 0 {
		t.Fatal("session retained call plans")
	}
	if _, err := bound.Call(abi.Int32(42)); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := bound.plan.Call(1, abi.Int32(42)); !errors.Is(err, abi.ErrClosed) {
		t.Fatal("native plan outlived session")
	}
}

func TestWindows386CallingConventions(t *testing.T) {
	if !abi.Available() {
		t.Skip("requires libffi")
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "386" {
		for _, convention := range []abi.Convention{abi.StdCall, abi.FastCall} {
			plan, err := abi.Prepare(abi.Signature{Convention: convention})
			if err == nil {
				plan.Close()
				t.Fatal("unsupported host convention accepted")
			}
		}
		return
	}
	needNative(t)
	p := compile(t, "testdata/conventions.c", filepath.Join(t.TempDir(), "conventions.obj"))
	s := New(Options{})
	defer s.Close()
	load(t, s, p)
	for _, tc := range []struct {
		name       string
		convention abi.Convention
	}{{"stdcall_add@8", abi.StdCall}, {"@fastcall_add@8", abi.FastCall}} {
		f, err := s.Bind(tc.name, abi.Signature{Convention: tc.convention, Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 100; i++ {
			v, err := f.Call(abi.Int32(-100), abi.Int32(142))
			if err != nil || v != abi.Int32(42) {
				t.Fatalf("%s: %+v %v", tc.name, v, err)
			}
		}
	}
}

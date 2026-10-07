package dylib

import (
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/cpunion/llgo-dylib/abi"
)

func TestBoundLifetime(t *testing.T) {
	needNative(t)
	p := compile(t, "testdata/add.c", filepath.Join(t.TempDir(), "add.o"))
	s := New(Options{})
	defer s.Close()
	load(t, s, p)
	f, e := s.BindInt32("add")
	if e != nil {
		t.Fatal(e)
	}
	v, e := f.Call(20, 22)
	if e != nil || v != 42 {
		t.Fatalf("%d %v", v, e)
	}
	s.Close()
	if _, e = f.Call(20, 22); !errors.Is(e, ErrClosed) {
		t.Fatalf("call after close: %v", e)
	}
}

func TestScalarABI(t *testing.T) {
	if !abi.Available() {
		t.Skip("requires -tags libffi")
	}
	needNative(t)
	p := compile(t, "testdata/scalars.c", filepath.Join(t.TempDir(), "scalars.o"))
	s := New(Options{})
	defer s.Close()
	load(t, s, p)
	f, e := s.Bind("mixed", abi.Signature{Result: abi.F64, Args: []abi.Type{abi.I32, abi.F64, abi.F32, abi.U64}})
	if e != nil {
		t.Fatal(e)
	}
	v, e := f.Call(abi.Int32(10), abi.Float64(20.5), abi.Float32(1.5), abi.Uint64(10))
	if e != nil || math.Float64frombits(v.Bits) != 42 {
		t.Fatalf("%+v %v", v, e)
	}
	if _, e = f.Call(abi.Int32(1)); e == nil {
		t.Fatal("bad signature accepted")
	}
	n, e := s.Bind("negate", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32}})
	if e != nil {
		t.Fatal(e)
	}
	v, e = n.Call(abi.Int32(42))
	if e != nil || int32(v.Bits) != -42 {
		t.Fatalf("signed return: %+v %v", v, e)
	}
	w, e := s.Bind("wide", abi.Signature{Result: abi.U64, Args: []abi.Type{abi.U64}})
	if e != nil {
		t.Fatal(e)
	}
	v, e = w.Call(abi.Uint64(0xffff000000000001))
	if e != nil || v.Bits != (uint64(0xffff000000000001)^0xfeed123456789abc) {
		t.Fatalf("wide return: %+v %v", v, e)
	}
	s.Close()
	if _, e = f.Call(); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
}

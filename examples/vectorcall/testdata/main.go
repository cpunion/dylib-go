package main

import (
	"errors"
	"fmt"
	dylib "github.com/cpunion/dylib-go"
	"os"
	"unsafe"
)

func check[T comparable](got T, err error, want T) {
	if err != nil || got != want {
		panic(fmt.Sprintf("vectorcall: %v, %v; want %v", got, err, want))
	}
}
func main() {
	s := dylib.New(dylib.Options{})
	defer s.Close()
	if err := s.Load(os.Args[1]); err != nil {
		panic(err)
	}
	b, err := NewBindings(s)
	if err != nil {
		panic(err)
	}
	// Repeat stack-argument calls to detect x86 cleanup mismatches.
	for i := 0; i < 100; i++ {
		u, e := b.Vector_unsigned(4294967295)
		check(u, e, uint32(4294967294))
		boolean, e := b.Vector_bool(false)
		check(boolean, e, true)
		boolean, e = b.Vector_bool(true)
		check(boolean, e, false)
		if err := b.Vector_void(); err != nil {
			panic(err)
		}
		v, e := b.Vector_zero()
		check(v, e, int32(42))
		v, e = b.Vector_add(-20, 62)
		check(v, e, int32(42))
		wide, e := b.Vector_wide(-128, 255, -32768, 65535, -9223372036854775807, 9223372036854742955)
		check(wide, e, int64(42))
		mixed, e := b.Vector_mixed(1, 2.5, 3.5, 4, 5.5, 6.5, 7, 8, 4)
		check(mixed, e, float64(42))
		pair, e := b.Vector_pair(BindingsRecord0{A: 20, B: 22})
		check(pair, e, BindingsRecord0{A: 21, B: 21})
		hfa, e := b.Vector_hfa(BindingsRecord1{A: 1, B: 2, C: 3, D: 4})
		check(hfa, e, BindingsRecord1{A: 2, B: 4, C: 6, D: 8})
		large, e := b.Vector_large(BindingsRecord2{A: [8]float64{20, 1, 2, 3, 4, 5, 6, 22}})
		check(large, e, BindingsRecord2{A: [8]float64{21, 1, 2, 3, 4, 5, 6, 21}})
		p, e := b.Vector_pointer(nil)
		check(p, e, unsafe.Pointer(nil))
	}
	factory, err := s.Resolve("vector_factory")
	if err != nil {
		panic(err)
	}
	// Native entries belong to this image; retain its lease across forwarding.
	err = factory.WithAddress(func(_ uintptr) error {
		entry, e := b.Vector_factory()
		if e != nil || entry == nil {
			panic(e)
		}
		p, e := b.Vector_pointer(entry)
		check(p, e, entry)
		hfaEntry, e := b.Vector_hfa_factory()
		if e != nil || hfaEntry == nil {
			panic(e)
		}
		for i := 0; i < 100; i++ {
			v, e := b.Vector_forward(entry, 20, 22)
			check(v, e, int32(42))
			v, e = b.Vector_apply(entry, 20, 22)
			check(v, e, int32(42))
			hfa, e := b.Vector_hfa_forward(hfaEntry, BindingsRecord1{A: 1, B: 2, C: 3, D: 4})
			check(hfa, e, BindingsRecord1{A: 2, B: 4, C: 6, D: 8})
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	if err := s.Close(); err != nil {
		panic(err)
	}
	if _, err := b.Vector_add(20, 22); !errors.Is(err, dylib.ErrClosed) {
		panic("closed vectorcall binding")
	}
	if _, err := b.Vector_factory(); !errors.Is(err, dylib.ErrClosed) {
		panic("closed vectorcall factory")
	}
	fmt.Println(42)
}

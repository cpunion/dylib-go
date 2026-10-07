// Package abi describes C ABI scalar signatures independently of object-file
// formats. Dynamic calls are optional: build with -tags libffi. For statically
// known signatures llgo can call a typed C function pointer directly.
package abi

import (
	"errors"
	"fmt"
	"math"
)

type Type uint8

const (
	Void Type = iota
	I32
	U32
	I64
	U64
	F32
	F64
	Pointer
)

// Signature uses the current host's default C calling convention. It does not
// describe variadic functions, aggregate values, C++ methods or Swift ABI calls.
type Signature struct {
	Result Type
	Args   []Type
}

func (s Signature) Validate() error {
	if s.Result > Pointer {
		return fmt.Errorf("invalid result type %d", s.Result)
	}
	if len(s.Args) > 32 {
		return fmt.Errorf("at most 32 arguments supported")
	}
	for _, t := range s.Args {
		if t == Void || t > Pointer {
			return fmt.Errorf("invalid argument type %d", t)
		}
	}
	return nil
}

// Value carries a scalar's exact bit pattern. Pointer values must refer to
// native-owned storage; this API does not pin Go objects or transfer ownership.
type Value struct {
	Type Type
	Bits uint64
}

func Int32(v int32) Value     { return Value{I32, uint64(uint32(v))} }
func Uint32(v uint32) Value   { return Value{U32, uint64(v)} }
func Int64(v int64) Value     { return Value{I64, uint64(v)} }
func Uint64(v uint64) Value   { return Value{U64, v} }
func Float32(v float32) Value { return Value{F32, uint64(math.Float32bits(v))} }
func Float64(v float64) Value { return Value{F64, math.Float64bits(v)} }
func Ptr(v uintptr) Value     { return Value{Pointer, uint64(v)} }

var ErrUnavailable = errors.New("dynamic C ABI calls require cgo and -tags libffi (plus libffi development files)")

// Call invokes a native function with a caller-supplied exact signature. It
// neither infers types from names nor adapts between operating-system ABIs.
// Prefer the owning Session.Bind API when using a dynamically loaded image.
func Call(address uintptr, s Signature, args ...Value) (Value, error) {
	if address == 0 {
		return Value{}, fmt.Errorf("null function address")
	}
	if err := s.Validate(); err != nil {
		return Value{}, err
	}
	if len(args) != len(s.Args) {
		return Value{}, fmt.Errorf("argument count mismatch")
	}
	for i, v := range args {
		if v.Type != s.Args[i] {
			return Value{}, fmt.Errorf("argument %d type mismatch", i)
		}
	}
	bits, err := invoke(address, s, args)
	return Value{Type: s.Result, Bits: bits}, err
}

package abi

import (
	"errors"
	"fmt"
	"math"
	"sync"
)

var ErrClosed = errors.New("ABI call plan is closed")

type callBackend interface {
	invoke(uintptr, Signature, []Value) (Value, error)
	close()
}

// CallPlan owns a prepared native call interface and its type layouts. It is
// independent of any code address; callers must keep the target image alive.
// Close releases native resources and waits for an active call. Do not copy it.
type CallPlan struct {
	mu        sync.Mutex
	signature Signature
	physical  Signature
	backend   callBackend
}

// Prepare validates and snapshots a concrete signature. Variadic tail values
// receive C default argument promotions; the fixed prefix is left unchanged.
// Close the result explicitly. Session.Bind manages plans automatically.
func Prepare(s Signature) (*CallPlan, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	s = s.Clone()
	physical := s.Clone()
	if s.Variadic {
		for i := s.FixedArgs; i < len(s.Args); i++ {
			switch s.Args[i] {
			case F32:
				physical.Args[i] = F64
			case Bool, I8, U8, I16, U16:
				physical.Args[i] = I32
			}
			if len(physical.ArgTypes) != 0 && physical.Args[i] != s.Args[i] {
				physical.ArgTypes[i] = TypeDesc{Type: physical.Args[i]}
			}
		}
	}
	b, err := prepare(physical)
	if err != nil {
		return nil, err
	}
	return &CallPlan{signature: s, physical: physical, backend: b}, nil
}

// Call invokes an exact native address with the plan's original logical types.
// Calls through one plan are serialized. Native code must not retain temporary
// pointers or re-enter this plan; use separate plans for independent calls.
func (p *CallPlan) Call(address uintptr, args ...Value) (Value, error) {
	if p == nil {
		return Value{}, ErrClosed
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.backend == nil {
		return Value{}, ErrClosed
	}
	if address == 0 {
		return Value{}, fmt.Errorf("null function address")
	}
	if len(args) != len(p.signature.Args) {
		return Value{}, fmt.Errorf("argument count mismatch")
	}
	for i, v := range args {
		if err := v.Validate(p.signature.ArgumentType(i)); err != nil {
			return Value{}, fmt.Errorf("argument %d: %w", i+1, err)
		}
	}
	if p.signature.Variadic {
		args = append([]Value(nil), args...)
		for i := p.signature.FixedArgs; i < len(args); i++ {
			v := args[i]
			switch v.Type {
			case F32:
				args[i] = Float64(float64(math.Float32frombits(uint32(v.Bits))))
			case I8:
				args[i] = Int32(int32(int8(v.Bits)))
			case I16:
				args[i] = Int32(int32(int16(v.Bits)))
			case U8, Bool:
				args[i] = Int32(int32(uint8(v.Bits)))
			case U16:
				args[i] = Int32(int32(uint16(v.Bits)))
			}
		}
	}
	return p.backend.invoke(address, p.physical, args)
}

func (p *CallPlan) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.backend != nil {
		p.backend.close()
		p.backend = nil
		p.signature = Signature{}
		p.physical = Signature{}
	}
	return nil
}

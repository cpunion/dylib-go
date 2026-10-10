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

// CallPlan retains a prepared native call interface and its type layouts. Share
// can give other logical plans independent references to those resources. It is
// independent of any code address; callers must keep the target image alive.
// Concurrent and reentrant calls share its immutable prepared layout. Close
// releases native resources after active calls return. Do not copy it.
type CallPlan struct {
	mu        sync.Mutex
	cond      *sync.Cond
	calls     int
	closing   bool
	signature Signature
	physical  Signature
	backend   callBackend
}

// Prepare validates and snapshots a concrete signature. Variadic tail values
// receive C default argument promotions; the fixed prefix is left unchanged.
// Close the result explicitly. Session.Bind manages plans automatically.
func Prepare(s Signature) (*CallPlan, error) {
	s, physical, err := planSignatures(s)
	if err != nil {
		return nil, err
	}
	b, err := prepare(physical)
	if err != nil {
		return nil, err
	}
	return &CallPlan{signature: s, physical: physical, backend: b}, nil
}

func planSignatures(s Signature) (Signature, Signature, error) {
	if err := s.Validate(); err != nil {
		return Signature{}, Signature{}, err
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
	return s, physical, nil
}

// Call invokes an exact native address with the plan's original logical types.
// Calls have independent native storage and may overlap or reenter this plan.
// Synchronize shared pointees/native state. Native code must not retain
// temporary pointers or close this plan from one of its own calls.
func (p *CallPlan) Call(address uintptr, args ...Value) (Value, error) {
	if p == nil {
		return Value{}, ErrClosed
	}
	p.mu.Lock()
	if p.backend == nil {
		p.mu.Unlock()
		return Value{}, ErrClosed
	}
	backend, signature, physical := p.backend, p.signature, p.physical
	p.calls++
	p.mu.Unlock()
	defer p.releaseCall()
	if address == 0 {
		return Value{}, fmt.Errorf("null function address")
	}
	if len(args) != len(signature.Args) {
		return Value{}, fmt.Errorf("argument count mismatch")
	}
	for i, v := range args {
		if err := v.Validate(signature.ArgumentType(i)); err != nil {
			return Value{}, fmt.Errorf("argument %d: %w", i+1, err)
		}
	}
	if signature.Variadic {
		args = append([]Value(nil), args...)
		for i := signature.FixedArgs; i < len(args); i++ {
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
	return backend.invoke(address, physical, args)
}

// Close rejects new calls and waits for active calls before freeing the CIF.
// Call it outside handlers invoked by this plan, to avoid waiting for itself.
func (p *CallPlan) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	for p.closing {
		p.waitCalls()
	}
	if p.backend == nil {
		p.mu.Unlock()
		return nil
	}
	backend := p.backend
	p.backend, p.closing = nil, true
	for p.calls != 0 {
		p.waitCalls()
	}
	p.signature, p.physical = Signature{}, Signature{}
	p.mu.Unlock()
	backend.close()
	p.mu.Lock()
	p.closing = false
	if p.cond != nil {
		p.cond.Broadcast()
	}
	p.mu.Unlock()
	return nil
}

func (p *CallPlan) waitCalls() {
	if p.cond == nil {
		p.cond = sync.NewCond(&p.mu)
	}
	p.cond.Wait()
}

func (p *CallPlan) releaseCall() {
	p.mu.Lock()
	p.calls--
	if p.calls == 0 && p.cond != nil {
		p.cond.Broadcast()
	}
	p.mu.Unlock()
}

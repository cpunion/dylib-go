package abi

import (
	"fmt"
	"reflect"
	"sync"
)

// ABIShape returns independent metadata for comparing the current C backend's
// native call shapes on one host. It applies C tail promotions, normalizes
// Default/CDecl and omits field names/pointee descriptions. It preserves scalar
// kinds, aggregate structure, array lengths and variadic boundaries. This is
// not register allocation, a persistent identifier or a language compatibility
// test; keep the original signature for value validation and conversion.
func (s Signature) ABIShape() (Signature, error) {
	_, physical, err := planSignatures(s)
	if err != nil {
		return Signature{}, err
	}
	return abiShape(physical), nil
}

func abiShape(physical Signature) Signature {
	shape := Signature{Result: physical.Result, Args: append([]Type(nil), physical.Args...),
		Convention: physical.Convention, Variadic: physical.Variadic, FixedArgs: physical.FixedArgs}
	if shape.Convention == Default {
		shape.Convention = CDecl
	}
	result := abiTypeShape(physical.ReturnType())
	shape.ResultType = &result
	for i := range physical.Args {
		shape.ArgTypes = append(shape.ArgTypes, abiTypeShape(physical.ArgumentType(i)))
	}
	return shape
}

func abiTypeShape(description TypeDesc) TypeDesc {
	shape := TypeDesc{Type: description.Type, Len: description.Len}
	if description.Type == Array {
		element := abiTypeShape(*description.Elem)
		shape.Elem = &element
	}
	for _, field := range description.Fields {
		shape.Fields = append(shape.Fields, Field{Type: abiTypeShape(field.Type)})
	}
	return shape
}

// Share creates an independently closed logical plan using this plan's native
// resources. The signatures must have equal ABIShape values; sharing never
// changes either plan's validation, pointee contracts or result metadata. Calls
// may overlap/reenter and use separate buffers. Closing one plan leaves the
// others alive; the last Close releases native resources after its calls finish.
func (p *CallPlan) Share(signature Signature) (*CallPlan, error) {
	if p == nil {
		return nil, ErrClosed
	}
	logical, physical, err := planSignatures(signature)
	if err != nil {
		return nil, err
	}
	shape := abiShape(physical)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.backend == nil {
		return nil, ErrClosed
	}
	if !reflect.DeepEqual(shape, abiShape(p.physical)) {
		return nil, fmt.Errorf("ABI signatures have different native call shapes")
	}
	shared, ok := p.backend.(*sharedCallBackend)
	if !ok {
		shared = &sharedCallBackend{backend: p.backend, refs: 1}
		p.backend = shared
	}
	shared.mu.Lock()
	shared.refs++
	shared.mu.Unlock()
	return &CallPlan{signature: logical, physical: physical, backend: shared}, nil
}

type sharedCallBackend struct {
	mu      sync.Mutex
	refs    int
	backend callBackend
}

func (b *sharedCallBackend) invoke(address uintptr, signature Signature, args []Value) (Value, error) {
	// A calling plan retains its reference until all its invocations finish.
	return b.backend.invoke(address, signature, args)
}

func (b *sharedCallBackend) close() {
	b.mu.Lock()
	b.refs--
	var backend callBackend
	if b.refs == 0 {
		backend, b.backend = b.backend, nil
	}
	b.mu.Unlock()
	if backend != nil {
		backend.close()
	}
}

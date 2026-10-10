package abi

import (
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestABIShapeCanonicalization(t *testing.T) {
	scalar := Signature{Result: I32, Args: []Type{Pointer, I32}}
	explicit := scalar.Clone()
	explicit.Convention = CDecl
	explicit.ResultType = &TypeDesc{Type: I32}
	explicit.ArgTypes = []TypeDesc{{Type: Pointer, Elem: &TypeDesc{Type: F64}}, {Type: I32}}
	pair := TypeDesc{Type: Struct, Fields: []Field{{Name: "a", Type: TypeDesc{Type: I32}}, {Name: "b", Type: TypeDesc{Type: Pointer, Elem: &TypeDesc{Type: I32}}}}}
	array := TypeDesc{Type: Array, Len: 2, Elem: &pair}
	record := TypeDesc{Type: Struct, Fields: []Field{{Name: "pairs", Type: array}}}
	first := Signature{Result: Struct, ResultType: &record, Args: []Type{Struct}, ArgTypes: []TypeDesc{record}}
	second := first.Clone()
	second.ResultType.Fields[0].Name = "renamed"
	second.ArgTypes[0].Fields[0].Type.Elem.Fields[0].Name = "x"
	second.ArgTypes[0].Fields[0].Type.Elem.Fields[1].Type.Elem = &TypeDesc{Type: F64}
	for _, test := range []struct {
		name string
		a, b Signature
	}{
		{"scalar metadata and convention", scalar, explicit},
		{"nested names and pointees", first, second},
		{"empty metadata", Signature{Result: Void}, Signature{Result: Void, Args: []Type{}, ArgTypes: []TypeDesc{}}},
		{"float tail", Signature{Result: I32, Args: []Type{I32, F32}, Variadic: true, FixedArgs: 1}, Signature{Result: I32, Args: []Type{I32, F64}, Variadic: true, FixedArgs: 1}},
		{"integer tail", Signature{Result: I32, Args: []Type{I32, U16}, Variadic: true, FixedArgs: 1}, Signature{Result: I32, Args: []Type{I32, I32}, Variadic: true, FixedArgs: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, err := test.a.ABIShape()
			if err != nil {
				t.Fatal(err)
			}
			b, err := test.b.ABIShape()
			if err != nil || !reflect.DeepEqual(a, b) {
				t.Fatalf("compatible shapes differ: %+v, %+v, %v", a, b, err)
			}
		})
	}
	shape, err := first.ABIShape()
	if err != nil {
		t.Fatal(err)
	}
	shape.ArgTypes[0].Fields[0].Type.Elem.Fields[0].Type.Type = U64
	shape.ResultType.Fields[0].Type.Len = 3
	if first.ArgTypes[0].Fields[0].Type.Elem.Fields[0].Type.Type != I32 || record.Fields[0].Type.Len != 2 {
		t.Fatal("shape mutation reached the original signature")
	}
}

func TestABIShapePreservesCallBoundaries(t *testing.T) {
	base := Signature{Result: I32, Args: []Type{I32, F64}, Variadic: true, FixedArgs: 1}
	for _, modify := range []func(*Signature){
		func(s *Signature) { s.Result = U32 },
		func(s *Signature) { s.Args[0] = I64 },
		func(s *Signature) { s.Args[0] = U32 },
		func(s *Signature) { s.FixedArgs = 2 },
		func(s *Signature) { s.Variadic, s.FixedArgs = false, 0 },
		func(s *Signature) { s.Convention, s.Variadic, s.FixedArgs = StdCall, false, 0 },
	} {
		other := base.Clone()
		modify(&other)
		a, err := base.ABIShape()
		if err != nil {
			t.Fatal(err)
		}
		b, err := other.ABIShape()
		if err != nil || reflect.DeepEqual(a, b) {
			t.Fatalf("different call shapes conflated: %+v, %v", other, err)
		}
	}
	for _, kind := range []Type{Bool, U8, I8, I16, U16, I32, U32, I64, U64, F32, F64} {
		shape, err := (Signature{Result: kind, Args: []Type{kind}}).ABIShape()
		if err != nil || shape.Result != kind || shape.Args[0] != kind {
			t.Fatalf("fixed scalar kind changed: %d, %v", kind, err)
		}
	}
	bad := TypeDesc{Type: Struct, Fields: []Field{{Name: "duplicate", Type: TypeDesc{Type: I32}}, {Name: "duplicate", Type: TypeDesc{Type: I32}}}}
	if _, err := (Signature{Result: Struct, ResultType: &bad}).ABIShape(); err == nil {
		t.Fatal("invalid metadata was erased before validation")
	}
	array := TypeDesc{Type: Array, Len: 2, Elem: &TypeDesc{Type: I32}}
	record := TypeDesc{Type: Struct, Fields: []Field{{Type: array}, {Type: TypeDesc{Type: U64}}}}
	aggregate := Signature{Result: Struct, ResultType: &record}
	for _, modify := range []func(*TypeDesc){
		func(d *TypeDesc) { d.Fields[0].Type.Len = 3 },
		func(d *TypeDesc) { d.Fields[0], d.Fields[1] = d.Fields[1], d.Fields[0] },
		func(d *TypeDesc) { d.Fields[0].Type.Elem.Type = F32 },
	} {
		other := aggregate.Clone()
		modify(other.ResultType)
		a, _ := aggregate.ABIShape()
		b, err := other.ABIShape()
		if err != nil || reflect.DeepEqual(a, b) {
			t.Fatalf("different aggregate layouts conflated: %+v, %v", other, err)
		}
	}
}

func TestSharedPlanLogicalValidationAndLifetime(t *testing.T) {
	signature := Signature{Result: F64, Args: []Type{I32, F32}, Variadic: true, FixedArgs: 1}
	logical, physical, err := planSignatures(signature)
	if err != nil {
		t.Fatal(err)
	}
	var cleanups atomic.Int32
	first := &CallPlan{signature: logical, physical: physical, backend: &testCallBackend{call: func(args []Value) (Value, error) { return args[1], nil }, cleanup: func() { cleanups.Add(1) }}}
	defer first.Close()
	signature.Args[1] = F64
	second, err := first.Share(signature)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	signature.Args[1] = I32
	if _, err := first.Share(signature); err == nil {
		t.Fatal("incompatible call shape was shared")
	}
	if _, err := first.Call(1, Int32(1), Float64(42)); err == nil {
		t.Fatal("sharing changed the original logical types")
	}
	if _, err := second.Call(1, Int32(1), Float32(42)); err == nil {
		t.Fatal("shared plan accepted the other logical type")
	}
	if result, err := first.Call(1, Int32(1), Float32(42)); err != nil || result != Float64(42) {
		t.Fatalf("original promotion: %+v, %v", result, err)
	}
	first.Close()
	if cleanups.Load() != 0 {
		t.Fatal("closing the original freed shared resources")
	}
	if _, err := first.Share(second.signature); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed owner shared resources: %v", err)
	}
	third, err := second.Share(second.signature)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	second.Close()
	if result, err := third.Call(1, Int32(1), Float64(42)); err != nil || result != Float64(42) {
		t.Fatalf("surviving plan: %+v, %v", result, err)
	}
	third.Close()
	third.Close()
	if cleanups.Load() != 1 {
		t.Fatalf("native resource cleanup count: %d", cleanups.Load())
	}
	if _, err := (*CallPlan)(nil).Share(Signature{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("nil owner: %v", err)
	}
}

func TestSharedPlanConcurrentCallsAndRetirement(t *testing.T) {
	signature := Signature{Result: I32, Args: []Type{I32}}
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var cleanups atomic.Int32
	first := &CallPlan{signature: signature, physical: signature, backend: &testCallBackend{call: func(args []Value) (Value, error) {
		entered <- struct{}{}
		<-release
		return args[0], nil
	}, cleanup: func() { cleanups.Add(1) }}}
	defer first.Close()
	second, err := first.Share(signature)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		releaseOnce.Do(func() { close(release) })
		second.Close()
	}()
	results := make(chan error, 2)
	for _, plan := range []*CallPlan{first, second} {
		go func(plan *CallPlan) {
			got, err := plan.Call(1, Int32(42))
			if err == nil && got != Int32(42) {
				err = errors.New("overlapping call changed result")
			}
			results <- err
		}(plan)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			releaseOnce.Do(func() { close(release) })
			t.Fatal("shared plans serialized native invocations")
		}
	}
	closed := make(chan error, 3)
	go func() { closed <- first.Close() }()
	go func() { closed <- second.Close() }()
	go func() { closed <- second.Close() }()
	for _, plan := range []*CallPlan{first, second} {
		deadline := time.Now().Add(5 * time.Second)
		for {
			plan.mu.Lock()
			retired := plan.backend == nil
			plan.mu.Unlock()
			if retired {
				break
			}
			if time.Now().After(deadline) {
				releaseOnce.Do(func() { close(release) })
				t.Fatal("shared plan did not retire")
			}
			runtime.Gosched()
		}
	}
	if cleanups.Load() != 0 {
		t.Fatal("shared backend freed during active calls")
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := <-closed; err != nil {
			t.Fatal(err)
		}
	}
	if cleanups.Load() != 1 {
		t.Fatalf("shared cleanup count: %d", cleanups.Load())
	}
}

func TestShareDuringCallAndCrossPlanReentry(t *testing.T) {
	signature := Signature{Result: I32, Args: []Type{I32}}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	var second *CallPlan
	var cleanups atomic.Int32
	first := &CallPlan{signature: signature, physical: signature, backend: &testCallBackend{call: func(args []Value) (Value, error) {
		if args[0] == Int32(0) {
			close(entered)
			<-release
			return second.Call(1, Int32(42))
		}
		if args[0] == Int32(-1) {
			panic("shared call panic")
		}
		return args[0], nil
	}, cleanup: func() { cleanups.Add(1) }}}
	defer func() {
		releaseOnce.Do(func() { close(release) })
		first.Close()
		second.Close()
	}()
	result := make(chan error, 1)
	go func() {
		got, err := first.Call(1, Int32(0))
		if err == nil && got != Int32(42) {
			err = errors.New("cross-plan reentry changed result")
		}
		result <- err
	}()
	<-entered // The old backend is already captured by the original call.
	var err error
	second, err = first.Share(signature)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- first.Close() }()
	releaseOnce.Do(func() { close(release) })
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil || cleanups.Load() != 0 {
		t.Fatalf("retiring the original broke a shared invocation: %v", err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "shared call panic" {
				t.Errorf("shared panic changed: %v", recovered)
			}
		}()
		second.Call(1, Int32(-1))
	}()
	second.Close()
	if cleanups.Load() != 1 {
		t.Fatal("panic retained an active shared call")
	}
}

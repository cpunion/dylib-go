package dylib

import (
	"errors"
	"sync"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestNativeBindingCacheAddressesAndOwnership(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			s := nativeABILibrary(t, "testdata/many_arguments.c", input)
			sig := abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}
			add, err := s.Bind("sum_two", sig)
			if err != nil {
				t.Fatal(err)
			}
			sub, err := s.Bind("sub_two", sig.Clone())
			if err != nil {
				t.Fatal(err)
			}
			if add.plan != sub.plan || add.symbol.address == sub.symbol.address || len(s.plans) != 1 || len(s.physicalPlans) != 1 {
				t.Fatal("equal signatures did not share only their call plan")
			}
			if _, err := s.Bind("missing_cache_symbol", sig); err == nil || len(s.plans) != 1 {
				t.Fatal("missing symbol changed the cache")
			}
			var wg sync.WaitGroup
			for _, f := range []*Function{add, sub} {
				wg.Add(1)
				go func(f *Function) {
					defer wg.Done()
					for i := 0; i < 20; i++ {
						a := int32(20)
						if f == sub {
							a = 64
						}
						if got, err := f.Call(abi.Int32(a), abi.Int32(22)); err != nil || got != abi.Int32(42) {
							t.Errorf("cached symbol call: %+v, %v", got, err)
							return
						}
					}
				}(f)
			}
			wg.Wait()
			// Changing the input snapshot cannot poison the cache or old plans.
			sig.Args[0] = abi.Void
			if _, err := s.Bind("sum_two", sig); err == nil || len(s.plans) != 1 {
				t.Fatal("invalid signature reused or changed the cache")
			}
			sig.Args[0] = abi.I32
			again, err := s.Bind("sum_two", sig)
			if err != nil || again.plan != add.plan {
				t.Fatalf("signature snapshot was mutated: %v", err)
			}
			other := nativeABILibrary(t, "testdata/many_arguments.c", input)
			separate, err := other.Bind("sum_two", sig)
			if err != nil || separate.plan == add.plan {
				t.Fatalf("cache crossed session ownership: %v", err)
			}
			if err := s.Close(); err != nil || len(s.plans) != 0 || len(s.physicalPlans) != 0 {
				t.Fatalf("cache cleanup: %v", err)
			}
			if _, err := add.plan.Call(1, abi.Int32(20), abi.Int32(22)); !errors.Is(err, abi.ErrClosed) {
				t.Fatal("cache plan outlived its owner")
			}
			if got, err := separate.Call(abi.Int32(20), abi.Int32(22)); err != nil || got != abi.Int32(42) {
				t.Fatalf("closing one session invalidated another: %+v, %v", got, err)
			}
		})
	}
}

func TestNativeBindingCachePreservesLogicalMetadata(t *testing.T) {
	s := nativeABILibrary(t, "testdata/pair.c", "object")
	desc := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "a", Type: abi.TypeDesc{Type: abi.I32}}, {Name: "b", Type: abi.TypeDesc{Type: abi.I32}}}}
	sig := abi.Signature{Result: abi.Struct, ResultType: &desc, Args: []abi.Type{abi.Struct}, ArgTypes: []abi.TypeDesc{desc}}
	first, err := s.Bind("echo_pair", sig)
	if err != nil {
		t.Fatal(err)
	}
	desc.Fields[0].Name = "renamed"
	second, err := s.Bind("echo_pair", sig)
	if err != nil || first.plan == second.plan {
		t.Fatalf("distinct logical metadata was conflated: %v", err)
	}
	if len(s.plans) != 2 || len(s.physicalPlans) != 1 {
		t.Fatal("renamed fields did not share one physical call interface")
	}
	missing := sig.Clone()
	missing.ArgTypes[0].Fields[1].Name = "missing"
	if _, err := s.Bind("missing_shared_symbol", missing); err == nil || len(s.plans) != 2 || len(s.physicalPlans) != 1 {
		t.Fatal("failed shared binding published a logical or physical plan")
	}
	if _, err := s.Bind("missing_new_shape", abi.Signature{Result: abi.F64}); err == nil || len(s.plans) != 2 || len(s.physicalPlans) != 1 {
		t.Fatal("failed fresh binding published a physical plan")
	}
	v, err := abi.StructValue(desc, abi.Int32(20), abi.Int32(22))
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range []*Function{first, second} {
		got, err := f.Call(v)
		name := "a"
		if i == 1 {
			name = "renamed"
		}
		if err != nil || got.Aggregate == nil || got.Aggregate.Type.Fields[0].Name != name {
			t.Fatalf("cached result metadata: %+v, %v", got, err)
		}
	}
}

func TestNativeLargePointeeRemainsOpaque(t *testing.T) {
	s := nativeABILibrary(t, "testdata/many_arguments.c", "object")
	elem := abi.TypeDesc{Type: abi.I64}
	array := abi.TypeDesc{Type: abi.Array, Elem: &elem, Len: 65536}
	pointer := abi.TypeDesc{Type: abi.Pointer, Elem: &array}
	f, err := s.Bind("identity_pointer", abi.Signature{Result: abi.Pointer, ResultType: &pointer, Args: []abi.Type{abi.Pointer}, ArgTypes: []abi.TypeDesc{pointer}})
	if err != nil {
		t.Fatal("raw pointer required an oversized temporary layout:", err)
	}
	// The native identity function never dereferences this pointer.
	if got, err := f.Call(abi.Ptr(0x1234)); err != nil || got != abi.Ptr(0x1234) {
		t.Fatalf("opaque oversized pointee: %+v, %v", got, err)
	}
}

func TestNativeBindingCacheDistinguishesCallShapes(t *testing.T) {
	s := nativeABILibrary(t, "testdata/many_arguments.c", "object")
	first, err := s.Bind("answer", abi.Signature{Result: abi.I32, Args: []abi.Type{}, ArgTypes: []abi.TypeDesc{}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Bind("answer", abi.Signature{Result: abi.I32})
	if err != nil || first.plan != second.plan {
		t.Fatalf("empty arguments were not canonicalized: %v", err)
	}
	base := abi.Signature{Result: abi.F64, Args: []abi.Type{abi.I32, abi.F32}, Variadic: true, FixedArgs: 1}
	float32, err := s.Bind("sum_many_variadic", base)
	if err != nil {
		t.Fatal(err)
	}
	base.Args[1] = abi.F64 // Same physical promotion, different logical values.
	float64, err := s.Bind("sum_many_variadic", base)
	if err != nil || float32.plan == float64.plan {
		t.Fatalf("logical variadic types were conflated: %v", err)
	}
	if len(s.plans) != 3 || len(s.physicalPlans) != 2 {
		t.Fatal("promoted variadic tails did not share a physical call interface")
	}
	for i, f := range []*Function{float32, float64} {
		value := abi.Float32(42)
		if i == 1 {
			value = abi.Float64(42)
		}
		if got, err := f.Call(abi.Int32(1), value); err != nil || got != abi.Float64(42) {
			t.Fatalf("cached variadic promotion: %+v, %v", got, err)
		}
	}
	// Inspect these distinct preparations without calling an incompatible shape.
	base.FixedArgs = 2
	boundary, err := s.Bind("sum_many_variadic", base)
	if err != nil || boundary.plan == float64.plan {
		t.Fatalf("variadic boundaries were conflated: %v", err)
	}
	base.FixedArgs, base.Convention = 1, abi.CDecl
	convention, err := s.Bind("sum_many_variadic", base)
	if err != nil || convention.plan == float64.plan {
		t.Fatalf("conventions were conflated: %v", err)
	}
	if len(s.plans) != 5 || len(s.physicalPlans) != 3 {
		t.Fatal("default/cdecl sharing or variadic boundary isolation failed")
	}
}

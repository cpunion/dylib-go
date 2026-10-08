package dylib

import (
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

// Fixture compilation, loading, and binding are outside the measured loops.
// Run with -tags libffi; Go and llgo use the same benchmarks and native code.
func BenchmarkNativeCalls(b *testing.B) {
	s := nativeABILibrary(b, "testdata/many_arguments.c", "object")
	large, largeArgs, largeWant := manyScalarArguments()
	pair := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "a", Type: abi.TypeDesc{Type: abi.I32}}, {Name: "b", Type: abi.TypeDesc{Type: abi.I32}}}}
	value, err := abi.StructValue(pair, abi.Int32(20), abi.Int32(22))
	if err != nil {
		b.Fatal(err)
	}
	for _, shape := range []struct {
		name   string
		symbol string
		sig    abi.Signature
		args   []abi.Value
		want   abi.Value
	}{
		{"Scalar2", "sum_two", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}, []abi.Value{abi.Int32(20), abi.Int32(22)}, abi.Int32(42)},
		{"Scalar64", "sum_many", large, largeArgs, largeWant},
		{"Struct", "sum_pair", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.Struct}, ArgTypes: []abi.TypeDesc{pair}}, []abi.Value{value}, abi.Int32(42)},
		{"Pointer", "read_pair", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.Pointer}, ArgTypes: []abi.TypeDesc{{Type: abi.Pointer, Elem: &pair}}}, []abi.Value{abi.AddressOf(&value)}, abi.Int32(42)},
	} {
		b.Run(shape.name, func(b *testing.B) {
			f, err := s.Bind(shape.symbol, shape.sig)
			if err != nil {
				b.Fatal(err)
			}
			plan, err := abi.Prepare(shape.sig)
			if err != nil {
				b.Fatal(err)
			}
			defer plan.Close()
			entry, err := s.Resolve(shape.symbol)
			if err != nil {
				b.Fatal(err)
			}
			if err := entry.WithAddress(func(address uintptr) error {
				for _, mode := range []string{"Bound", "Prepared", "OneShot"} {
					b.Run(mode, func(b *testing.B) {
						b.ReportAllocs()
						for i := 0; i < b.N; i++ {
							var got abi.Value
							var err error
							switch mode {
							case "Bound":
								got, err = f.Call(shape.args...)
							case "Prepared":
								got, err = plan.Call(address, shape.args...)
							case "OneShot":
								got, err = abi.Call(address, shape.sig, shape.args...)
							}
							if err != nil || got != shape.want {
								b.Fatalf("native call: %+v, %v", got, err)
							}
						}
					})
				}
				return nil
			}); err != nil {
				b.Fatal(err)
			}
		})
	}
}

func BenchmarkNativeBindingCache(b *testing.B) {
	s := nativeABILibrary(b, "testdata/many_arguments.c", "object")
	sig := abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}
	first, err := s.Bind("sum_two", sig)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := s.Bind("sum_two", sig)
		if err != nil || f.plan != first.plan {
			b.Fatalf("cached binding: %v", err)
		}
	}
}

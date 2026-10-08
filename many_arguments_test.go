package dylib

import (
	"fmt"
	"math"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func manyScalarArguments() (abi.Signature, []abi.Value, abi.Value) {
	sig := abi.Signature{Result: abi.I64, Args: make([]abi.Type, 64)}
	args := make([]abi.Value, len(sig.Args))
	var sum int64
	for i := range args {
		v := int32(i + 1)
		sig.Args[i], args[i] = abi.I32, abi.Int32(v)
		sum += int64(i+1) * int64(v)
	}
	return sig, args, abi.Int64(sum)
}

func TestNativeLargeSignatures(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			s := nativeABILibrary(t, "testdata/many_arguments.c", input)
			sig, args, want := manyScalarArguments()
			f, err := s.Bind("sum_many", sig)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				got, err := f.Call(args...)
				if err != nil || got != want {
					t.Fatalf("64 scalar arguments: %+v, %v; want %+v", got, err, want)
				}
			}
			if _, err := f.Call(args[:63]...); err == nil {
				t.Fatal("wrong count accepted")
			}

			desc := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: "a", Type: abi.TypeDesc{Type: abi.I32}}, {Name: "b", Type: abi.TypeDesc{Type: abi.I32}}}}
			sig.ArgTypes = make([]abi.TypeDesc, 64)
			var sum int64
			for i := range args {
				a, b := int32(i+1), -int32(i)
				args[i], err = abi.StructValue(desc, abi.Int32(a), abi.Int32(b))
				if err != nil {
					t.Fatal(err)
				}
				sig.Args[i], sig.ArgTypes[i] = abi.Struct, desc
				sum += int64(i+1) * (3*int64(a) + int64(b))
			}
			f, err = s.Bind("sum_many_pairs", sig)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := f.Call(args...); err != nil || got != abi.Int64(sum) {
				t.Fatalf("64 struct arguments: %+v, %v; want %d", got, err, sum)
			}

			zero, err := s.Bind("answer", abi.Signature{Result: abi.I32})
			if err != nil {
				t.Fatal(err)
			}
			if got, err := zero.Call(); err != nil || got != abi.Int32(42) {
				t.Fatalf("zero arguments: %+v, %v", got, err)
			}
		})
	}
}

func TestNativeLargeVariadicShapes(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			s := nativeABILibrary(t, "testdata/many_arguments.c", input)
			// Total counts straddle the former 32-slot scalar scratch buffer.
			for _, tail := range []int{0, 31, 32, 63} {
				t.Run(fmt.Sprint(tail+1), func(t *testing.T) {
					sig := abi.Signature{Result: abi.F64, Args: []abi.Type{abi.I32}, Variadic: true, FixedArgs: 1}
					args := []abi.Value{abi.Int32(int32(tail))}
					var sum float64
					for i := 0; i < tail; i++ {
						var v abi.Value
						var number float64
						switch i % 3 {
						case 0:
							number = float64(i) + 0.5
							v = abi.Float32(float32(number))
						case 1:
							number = -float64(i)
							v = abi.Int8(int8(-i))
						case 2:
							number = float64(60000 + i)
							v = abi.Uint16(uint16(number))
						}
						sig.Args, args = append(sig.Args, v.Type), append(args, v)
						sum += float64(i+1) * number
					}
					f, err := s.Bind("sum_many_variadic", sig)
					if err != nil {
						t.Fatal(err)
					}
					got, err := f.Call(args...)
					if err != nil || got.Type != abi.F64 || math.Float64frombits(got.Bits) != sum {
						t.Fatalf("%d variadic values: %+v, %v; want %g", tail, got, err, sum)
					}
				})
			}
		})
	}
}

func TestNativeLargeCallbackSignature(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			s := nativeABILibrary(t, "testdata/many_arguments.c", input)
			sig, wantArgs, want := manyScalarArguments()
			cb, err := abi.NewCallback(sig, func(args []abi.Value) (abi.Value, error) {
				if len(args) != len(wantArgs) {
					return abi.Value{}, fmt.Errorf("callback count: %d", len(args))
				}
				var sum int64
				for i, v := range args {
					if v != wantArgs[i] {
						return abi.Value{}, fmt.Errorf("callback argument %d: %+v", i, v)
					}
					sum += int64(i+1) * int64(int32(v.Bits))
				}
				return abi.Int64(sum), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer cb.Close()
			got := callbackCall(t, s, "func invoke_many(unsafe.Pointer)int64", cb)
			if got != want || cb.Err() != nil {
				t.Fatalf("64 callback arguments: %+v, callback=%v", got, cb.Err())
			}
		})
	}
}

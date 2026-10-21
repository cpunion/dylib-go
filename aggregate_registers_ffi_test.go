//go:build libffi && cgo && (linux || darwin || windows)

package dylib

import (
	"reflect"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
	"github.com/ramblelabs/dylib-go/abi/signature"
)

func TestNativeDynamicAggregateRegisters(t *testing.T) {
	withAggregateRegisters(t, func(t *testing.T, s *Session) {
		prefix := func(n int, kind abi.Type) []abi.Value {
			values := make([]abi.Value, n)
			for i := range values {
				if kind == abi.F64 {
					values[i] = abi.Float64(float64(i + 1))
				} else {
					values[i] = abi.Int64(int64(i + 1))
				}
			}
			return values
		}
		record := func(kinds []abi.Type, values ...abi.Value) abi.Value {
			d := abi.TypeDesc{Type: abi.Struct}
			for i, kind := range kinds {
				d.Fields = append(d.Fields, abi.Field{Name: string(rune('a' + i)), Type: abi.TypeDesc{Type: kind}})
			}
			v, err := abi.StructValue(d, values...)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		floatPair := record([]abi.Type{abi.F32, abi.F32}, abi.Float32(1), abi.Float32(2))
		mixed := record([]abi.Type{abi.F64, abi.I64}, abi.Float64(1), abi.Int64(2))
		floating := func(shape string) abi.Value {
			d, err := signature.Parse("func value()" + shape)
			if err != nil {
				t.Fatal(err)
			}
			v := abi.Zero(d.Signature.ReturnType())
			var fill func(*abi.Value)
			fill = func(v *abi.Value) {
				if v.Aggregate != nil {
					for i := range v.Aggregate.Fields {
						fill(&v.Aggregate.Fields[i])
					}
				} else if v.Type == abi.F32 {
					*v = abi.Float32(1)
				} else {
					*v = abi.Float64(1)
				}
			}
			fill(&v)
			return v
		}
		for _, tc := range []struct {
			declaration string
			args        []abi.Value
			want        abi.Value
		}{
			{"func exhausted_float(float64,float64,float64,float64,float64,float64,float64,float64,struct{a,b float32},float64)float64", append(prefix(8, abi.F64), floatPair, abi.Float64(3)), abi.Float64(42)},
			{"func partial_float(float64,float64,float64,float64,float64,float64,float64,struct{a,b float32},float64)float64", append(prefix(7, abi.F64), record([]abi.Type{abi.F32, abi.F32}, abi.Float32(6), abi.Float32(7)), abi.Float64(1)), abi.Float64(42)},
			{"func partial_integer(int64,int64,int64,int64,int64,struct{a,b int64},int64)int64", append(prefix(5, abi.I64), record([]abi.Type{abi.I64, abi.I64}, abi.Int64(6), abi.Int64(7)), abi.Int64(14)), abi.Int64(42)},
			{"func rollback_integer(int64,int64,int64,int64,int64,int64,struct{a float64;b int64},float64)float64", append(prefix(6, abi.I64), record([]abi.Type{abi.F64, abi.I64}, abi.Float64(7), abi.Int64(8)), abi.Float64(6)), abi.Float64(42)},
			{"func rollback_float(float64,float64,float64,float64,float64,float64,float64,float64,struct{a float64;b int64},int64)float64", append(prefix(8, abi.F64), mixed, abi.Int64(3)), abi.Float64(42)},
			{"func stacked_triple(float64,float64,float64,float64,float64,float64,float64,float64,float32,struct{a float32;rest [2]float32},float64)float64", append(prefix(8, abi.F64), abi.Float32(1), floating("struct{a float32;rest [2]float32}"), abi.Float64(2)), abi.Float64(42)},
			{"func stacked_quad(float64,float64,float64,float64,float64,float64,float64,float64,struct{values [4]float64},float64)float64", append(prefix(8, abi.F64), floating("struct{values [4]float64}"), abi.Float64(2)), abi.Float64(42)},
			{"func hidden_result(int64,int64,int64,int64,int64,struct{a,b int32},int64)struct{a,b,c int64}", append(prefix(5, abi.I64), record([]abi.Type{abi.I32, abi.I32}, abi.Int32(6), abi.Int32(7)), abi.Int64(14)), record([]abi.Type{abi.I64, abi.I64, abi.I64}, abi.Int64(15), abi.Int64(13), abi.Int64(14))},
		} {
			d, err := signature.Parse(tc.declaration)
			if err != nil {
				t.Fatal(err)
			}
			t.Run(d.Name, func(t *testing.T) {
				fn, err := s.Bind(d.Name, d.Signature)
				if err != nil {
					t.Fatal(err)
				}
				got, err := fn.Call(tc.args...)
				if err != nil || !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
				}
			})
		}
	})
}

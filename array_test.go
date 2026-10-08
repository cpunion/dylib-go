package dylib

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
	"github.com/cpunion/dylib-go/abi/signature"
)

func TestNativeArrays(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			s := nativeABILibrary(t, "testdata/arrays.c", input)
			for _, tc := range []struct{ name, typ, literal string }{
				{"array_i32", "struct{values [2]int32}", "{{20,22}}"},
				{"array_f32", "struct{values [4]float32}", "{{10.5,10.5,10.5,10.5}}"},
				{"array_f64", "struct{values [2]float64}", "{{20.5,21.5}}"},
				{"array_padded", "struct{tag int8;values [3]int16;extra float64}", "{1,{10,11,19},1}"},
				{"array_records", "struct{items [2]struct{tag int8;value float64};tail int32}", "{{{1,10},{2,20}},9}"},
				{"array_matrix", "struct{values [2][3]int32}", "{{{1,2,3},{10,11,15}}}"},
				{"array_bytes", "struct{values [40]uint8}", "{{20,22}}"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					decl := callbackSignature(t, "func cb("+tc.typ+")"+tc.typ)
					value, err := signature.ParseTypedValue(decl.ArgumentType(0), tc.literal)
					if err != nil {
						t.Fatal(err)
					}
					sum, err := s.Bind("sum_"+tc.name, callbackSignature(t, "func sum("+tc.typ+")float64"))
					if err != nil {
						t.Fatal(err)
					}
					if got, err := sum.Call(value); err != nil || got != abi.Float64(42) {
						t.Fatalf("C layout: %+v, %v", got, err)
					}
					echo, err := s.Bind("echo_"+tc.name, decl)
					if err != nil {
						t.Fatal(err)
					}
					if got, err := echo.Call(value); err != nil || !reflect.DeepEqual(got, value) {
						t.Fatalf("roundtrip: %+v, %v", got, err)
					}
					called := false
					cb, err := abi.NewCallback(decl, func(args []abi.Value) (abi.Value, error) {
						called = true
						if !reflect.DeepEqual(args[0], value) {
							return abi.Value{}, errors.New("C-created array decoded incorrectly")
						}
						return value, nil
					})
					if err != nil {
						t.Fatal(err)
					}
					defer cb.Close()
					if got := callbackCall(t, s, "func callback_"+tc.name+"(unsafe.Pointer)float64", cb); !called || got != abi.Float64(42) || cb.Err() != nil {
						t.Fatalf("callback C layout: %+v, called=%v, %v", got, called, cb.Err())
					}
				})
			}
			call, err := signature.ParseCall("var_array(2:int32,{{20,20}}:struct{values [2]int32})int32")
			if err != nil {
				t.Fatal(err)
			}
			call.Signature.Variadic, call.Signature.FixedArgs = true, 1
			f, err := s.Bind(call.Name, call.Signature)
			if err != nil {
				t.Fatal(err)
			}
			if v, err := f.Call(call.Args...); err != nil || v != abi.Int32(42) {
				t.Fatalf("array in variadic record: %+v, %v", v, err)
			}
		})
	}
}

func TestNativeArrayPointers(t *testing.T) {
	for _, input := range []string{"object", "archive", "library"} {
		t.Run(input, func(t *testing.T) {
			s := nativeABILibrary(t, "testdata/arrays.c", input)
			for _, tc := range []struct{ text, want string }{
				{"sum_array_ptr(&{20,22}:*[2]int32)int32", "42"},
				{"mutate_array(&{20,22}:*[2]int32)*[2]int32", "&{22,20}"},
				{"mutate_array_refs({{&20,&22}}:struct{values [2]*int32})struct{values [2]*int32}", "{values:{&22,&20}}"},
			} {
				call, err := signature.ParseCall(tc.text)
				if err != nil {
					t.Fatal(err)
				}
				f, err := s.Bind(call.Name, call.Signature)
				if err != nil {
					t.Fatal(err)
				}
				v, err := f.Call(call.Args...)
				if err != nil {
					t.Fatal(err)
				}
				got, err := signature.FormatValue(v)
				if err != nil || got != tc.want {
					t.Fatalf("%s: got %s, %v; want %s", tc.text, got, err, tc.want)
				}
				if call.Name == "mutate_array" {
					updated, err := signature.FormatValue(call.Args[0])
					if err != nil || updated != tc.want || v.Pointee != call.Args[0].Pointee {
						t.Fatalf("array copy-back/identity: %s, %v", updated, err)
					}
				}
			}
			for _, tc := range []struct{ name, message string }{
				{"interior_array", "interior pointer"}, {"first_array", "incompatible pointee"},
			} {
				call, err := signature.ParseCall(tc.name + "(&{20,22}:*[2]int32)*int32")
				if err != nil {
					t.Fatal(err)
				}
				f, err := s.Bind(call.Name, call.Signature)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.Call(call.Args...); err == nil || !strings.Contains(err.Error(), tc.message) {
					t.Fatalf("%s: expected lifetime error, got %v", tc.name, err)
				}
			}
			call, err := signature.ParseCall("same_array(&{20,22}:*[2]int32,nil:*[2]int32)int32")
			if err != nil {
				t.Fatal(err)
			}
			f, err := s.Bind(call.Name, call.Signature)
			if err != nil {
				t.Fatal(err)
			}
			call.Args[1] = call.Args[0]
			// The prepared signature must own the array and its element metadata.
			call.Signature.ArgTypes[0].Elem.Len = 1
			call.Signature.ArgTypes[0].Elem.Elem.Type = abi.F64
			v, err := f.Call(call.Args...)
			if err != nil || v != abi.Int32(1) || call.Args[0].Pointee.Aggregate.Fields[0] != abi.Int32(22) {
				t.Fatalf("array alias or signature snapshot: %+v, %v", v, err)
			}
			for _, temporary := range []bool{false, true} {
				cb, err := abi.NewCallback(callbackSignature(t, "func cb(struct{values [2]*int32})struct{values [2]*int32}"), func(args []abi.Value) (abi.Value, error) {
					if temporary {
						v := abi.Int32(42)
						args[0].Aggregate.Fields[0].Aggregate.Fields[0] = abi.AddressOf(&v)
					}
					return args[0], nil
				})
				if err != nil {
					t.Fatal(err)
				}
				v := callbackCall(t, s, "func callback_array_refs(unsafe.Pointer)int32", cb)
				cb.Close()
				if (!temporary && (v != abi.Int32(42) || cb.Err() != nil)) || (temporary && (v != abi.Int32(-1) || cb.Err() == nil)) {
					t.Fatalf("callback array pointer lifetime: %+v, %v", v, cb.Err())
				}
			}
		})
	}
}

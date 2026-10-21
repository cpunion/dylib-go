package dylib

import (
	"math"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ramblelabs/dylib-go/abi"
)

func TestNativeAMD64Conventions(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("AMD64 calling conventions")
	}

	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			for _, input := range []string{"object", "archive", "library"} {
				t.Run(input, func(t *testing.T) {
					s := nativeABILibrary(t, "testdata/foreign_abi.c", input, opt)
					for _, c := range []struct {
						name       string
						convention abi.Convention
					}{{"ms", abi.Win64}, {"sysv", abi.SysV64}} {
						t.Run(c.name, func(t *testing.T) {
							if c.convention == abi.SysV64 && runtime.GOOS == "windows" {
								t.Skip("Windows libffi needs a compiled SysV bridge")
							}
							exerciseAMD64Convention(t, s, c.name, c.convention)
						})
					}
				})
			}
		})
	}

}

func exerciseAMD64Convention(t *testing.T, s *Session, prefix string, convention abi.Convention) {
	t.Helper()
	bind := func(name, decl string, cc abi.Convention) *Function {
		t.Helper()
		sig := callbackSignature(t, decl)
		sig.Convention = cc
		f, err := s.Bind(prefix+"_"+name, sig)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	invoke := func(f *Function, args ...abi.Value) abi.Value {
		t.Helper()
		v, err := f.Call(args...)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	t.Run("scalars", func(t *testing.T) {
		tests := []struct {
			name, decl string
			args       []abi.Value
			want       abi.Value
		}{
			{"add", "func f(int32,int32)int32", []abi.Value{abi.Int32(-20), abi.Int32(62)}, abi.Int32(42)},
			{"narrow", "func f(int8)int8", []abi.Value{abi.Int8(-120)}, abi.Int8(-120)},
			{"wide", "func f(uint64)uint64", []abi.Value{abi.Uint64(math.MaxUint64 - 7)}, abi.Uint64((math.MaxUint64 - 7) ^ 0x12345678fedcba98)},
			{"boolean", "func f(bool)bool", []abi.Value{abi.Boolean(true)}, abi.Boolean(false)},
			{"pointer", "func f(unsafe.Pointer)unsafe.Pointer", []abi.Value{abi.Ptr(0x12345678)}, abi.Ptr(0x12345678)},
			{"mixed", "func f(int8,uint16,float32,float64,int64)float64", []abi.Value{abi.Int8(-120), abi.Uint16(65000), abi.Float32(1.25), abi.Float64(-2.25), abi.Int64(1<<40 + 5)}, abi.Float64(-120 + 65000 + 1.25 - 2.25 + (1<<40 + 5))},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				f := bind(tc.name, tc.decl, convention)
				for i := 0; i < 100; i++ {
					if got := invoke(f, tc.args...); got != tc.want {
						t.Fatal(got, tc.want)
					}
				}
			})
		}
	})
	t.Run("register_spills", func(t *testing.T) {
		types := make([]string, 0, 18)
		args := make([]abi.Value, 0, 18)
		for i := 0; i < 8; i++ {
			types = append(types, "int32")
			args = append(args, abi.Int32(int32(i)))
		}
		for i := 0; i < 10; i++ {
			types = append(types, "float64")
			args = append(args, abi.Float64(float64(i)+0.5))
		}
		f := bind("spill", "func f("+strings.Join(types, ",")+")float64", convention)
		for i := 0; i < 100; i++ {
			if got := invoke(f, args...); got != abi.Float64(78) {
				t.Fatal(got)
			}
		}
	})
	t.Run("temporary_pointer", func(t *testing.T) {
		value := abi.Int32(20)
		f := bind("mutate", "func f(*int32,int32)", convention)
		if got := invoke(f, abi.AddressOf(&value), abi.Int32(22)); got.Type != abi.Void || value != abi.Int32(42) {
			t.Fatal(got, value)
		}
	})
	for _, shape := range []struct {
		name, decl    string
		initial, want []abi.Value
		tail          abi.Value
	}{
		{"pair", "struct{a int32;b float64}", []abi.Value{abi.Int32(20), abi.Float64(20.5)}, []abi.Value{abi.Int32(42), abi.Float64(42.5)}, abi.Int32(22)},
		{"large", "struct{a,b,c,d,e int64}", []abi.Value{abi.Int64(1), abi.Int64(2), abi.Int64(3), abi.Int64(4), abi.Int64(5)}, []abi.Value{abi.Int64(23), abi.Int64(24), abi.Int64(25), abi.Int64(26), abi.Int64(27)}, abi.Int64(22)},
	} {
		t.Run(shape.name, func(t *testing.T) {
			tailType := "int32"
			if shape.name == "large" {
				tailType = "int64"
			}
			decl := "func f(" + shape.decl + "," + tailType + ")" + shape.decl
			sig := callbackSignature(t, decl)
			sig.Convention = convention
			f := bind(shape.name, decl, convention)
			input, err := abi.StructValue(sig.ArgumentType(0), shape.initial...)
			if err != nil {
				t.Fatal(err)
			}
			want, err := abi.StructValue(sig.ReturnType(), shape.want...)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 100; i++ {
				if got := invoke(f, input, shape.tail); !reflect.DeepEqual(got, want) {
					t.Fatal(got, want)
				}
			}
			cb, err := abi.NewCallback(sig, func(args []abi.Value) (abi.Value, error) { return f.Call(args...) })
			if err != nil {
				t.Fatal(err)
			}
			defer cb.Close()
			applyName := "apply"
			if shape.name == "large" {
				applyName = "apply_large"
			}
			apply := bind(applyName, "func f(unsafe.Pointer,"+shape.decl+","+tailType+")"+shape.decl, abi.CDecl)
			err = cb.WithAddress(func(address uintptr) error {
				for i := 0; i < 100; i++ {
					if got := invoke(apply, abi.Ptr(address), input, shape.tail); !reflect.DeepEqual(got, want) {
						t.Fatal("callback:", got, want)
					}
				}
				return nil
			})
			if err != nil || cb.Err() != nil {
				t.Fatal(err, cb.Err())
			}
			if shape.name == "pair" {
				entry := invoke(bind("factory", "func f()unsafe.Pointer", abi.CDecl))
				symbol, err := s.Resolve(prefix + "_pair")
				if err != nil {
					t.Fatal(err)
				}
				plan, err := abi.Prepare(sig)
				if err != nil {
					t.Fatal(err)
				}
				defer plan.Close()
				if err := symbol.WithAddress(func(address uintptr) error {
					if address != uintptr(entry.Bits) {
						t.Fatal("factory identity")
					}
					got, err := plan.Call(uintptr(entry.Bits), input, shape.tail)
					if err != nil || !reflect.DeepEqual(got, want) {
						t.Fatal(got, err)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				forward := bind("forward", "func f(unsafe.Pointer,"+shape.decl+",int32)"+shape.decl, convention)
				if err := cb.WithAddress(func(address uintptr) error {
					if got := invoke(forward, abi.Ptr(address), input, shape.tail); !reflect.DeepEqual(got, want) {
						t.Fatal("nested convention:", got)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

package signature

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestDeclarations(t *testing.T) {
	uintptrType := abi.U64
	if strconv.IntSize == 32 {
		uintptrType = abi.U32
	}
	for _, tc := range []struct {
		text string
		name string
		sig  abi.Signature
	}{
		{"func add(int32,int32)int32", "add", abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}},
		{"func mixed(a int32, b float64, c float32, d uint64) float64", "mixed", abi.Signature{Result: abi.F64, Args: []abi.Type{abi.I32, abi.F64, abi.F32, abi.U64}}},
		{"func pair(a, b uint32) (result uint64)", "pair", abi.Signature{Result: abi.U64, Args: []abi.Type{abi.U32, abi.U32}}},
		{"func pointer(*int32, unsafe.Pointer) unsafe.Pointer", "pointer", abi.Signature{Result: abi.Pointer, Args: []abi.Type{abi.Pointer, abi.Pointer}}},
		{"func word(uintptr) uintptr", "word", abi.Signature{Result: uintptrType, Args: []abi.Type{uintptrType}}},
		{"func no_result()", "no_result", abi.Signature{}},
		{"func signed(int64) int64", "signed", abi.Signature{Result: abi.I64, Args: []abi.Type{abi.I64}}},
	} {
		t.Run(tc.text, func(t *testing.T) {
			d, err := Parse(tc.text)
			if err != nil || d.Name != tc.name || !reflect.DeepEqual(d.Signature, tc.sig) {
				t.Fatalf("got %+v, %v; want %s %+v", d, err, tc.name, tc.sig)
			}
		})
	}
	maximum := "func maximum(" + strings.Repeat("int32,", 32) + ")"
	if _, err := Parse(maximum); err != nil {
		t.Fatal(err)
	}
}

func TestRejectedDeclarations(t *testing.T) {
	for _, text := range []string{
		"", "func (int32) int32", "func _()", "func f(int) int32",
		"func f(uint) uint64", "func f(string)", "func f([]int32)",
		"func f(struct{ x int32 })", "func f(...int32)", "func f(func())",
		"func f() (int32,int32)", "func f() (a,b int32)", "func f() {}",
		"func (r T) f()", "func f[T any](T)", "var f int32",
		"func f(); func g()", "import \"unsafe\"; func f()",
		"func f(" + strings.Repeat("int32,", 33) + ")",
	} {
		t.Run(text, func(t *testing.T) {
			if d, err := Parse(text); err == nil {
				t.Fatalf("accepted %q as %+v", text, d)
			}
		})
	}
}

func TestTypedInvocations(t *testing.T) {
	for _, text := range []string{
		"add(20:int32 22:int32)int32",
		"add(20:int32,22:int32)int32",
		"add( 20 : int32 , 22 : int32 ) (int32)",
		"add(0x14:int32\n0b10110:int32)int32",
	} {
		call, err := ParseCall(text)
		if err != nil || call.Name != "add" || call.Signature.Result != abi.I32 || !reflect.DeepEqual(call.Args, []abi.Value{abi.Int32(20), abi.Int32(22)}) {
			t.Fatalf("%q: %+v, %v", text, call, err)
		}
	}
	call, err := ParseCall("mixed(-10:int32, 30.5:float64, 1.5:float32, 20:uint64)float64")
	if err != nil || !reflect.DeepEqual(call.Args, []abi.Value{abi.Int32(-10), abi.Float64(30.5), abi.Float32(1.5), abi.Uint64(20)}) {
		t.Fatalf("mixed call: %+v, %v", call, err)
	}
	call, err = ParseCall("echo_ptr(nil:unsafe . Pointer)unsafe.Pointer")
	if err != nil || len(call.Args) != 1 || call.Args[0] != abi.Ptr(0) {
		t.Fatalf("pointer call: %+v, %v", call, err)
	}
	call, err = ParseCall("no_result()")
	if err != nil || len(call.Args) != 0 || call.Signature.Result != abi.Void {
		t.Fatalf("void call: %+v, %v", call, err)
	}
	call, err = ParseCall("answer() (int32)")
	if err != nil || len(call.Args) != 0 || call.Signature.Result != abi.I32 {
		t.Fatalf("parenthesized result: %+v, %v", call, err)
	}
}

func TestRejectedInvocations(t *testing.T) {
	for _, text := range []string{
		"add", "(20:int32)int32", "add(20:int32", "add(20)int32",
		"add(:int32)int32", "add(20:)int32", "add(20:int32,,22:int32)int32",
		"add(20:int3222:int32)int32", "add(20:a,b int32)int32",
		"add(1+2:int32)int32", "add(2147483648:int32)int32",
		"add(-1:uint32)uint32", "add(20:string)int32", "add() (int32,int32)",
	} {
		t.Run(text, func(t *testing.T) {
			if call, err := ParseCall(text); err == nil {
				t.Fatalf("accepted %q as %+v", text, call)
			}
		})
	}
}

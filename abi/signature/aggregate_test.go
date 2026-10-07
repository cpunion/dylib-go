package signature

import (
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestStructLiterals(t *testing.T) {
	declaration, err := Parse("func f(struct{a int32;b int32})struct{a int32;b int32}")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"{20,22}", "{a:20,b:22}", "{b:22,a:20}"} {
		v, err := ParseTypedValue(declaration.Signature.ArgumentType(0), text)
		if err != nil {
			t.Fatal(err)
		}
		printed, err := FormatValue(v)
		if err != nil || printed != "{a:20,b:22}" {
			t.Fatalf("%s: %s %v", text, printed, err)
		}
	}
	for _, text := range []string{"{a:20,a:22}", "{unknown:20}", "{20}", "{20,b:22}", "{a:20,22}", "{a:2147483648}"} {
		if _, err := ParseTypedValue(declaration.Signature.ArgumentType(0), text); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
	for _, text := range []string{
		"sum_pair({a:20,b:22}:struct{a int32;b int32})int32",
		"sum_pair({20,22}:struct{a,b int32})int32",
		"sum_pair_ptr(&{a:20,b:22}:*struct{a int32;b int32})int32",
		"sum_nested({tag:1,p:{a:20,b:20},extra:1}:struct{tag int8;p struct{a,b int32};extra float64})float64",
		"echo_pair_ref({p:& {a:20,b:20}}:struct{p *struct{a,b int32};bonus int32})struct{p *struct{a,b int32};bonus int32}",
	} {
		call, err := ParseCall(text)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if err := call.Args[0].Validate(call.Signature.ArgumentType(0)); err != nil {
			t.Fatal(err)
		}
	}
	call, err := ParseCall("scalar_ptr(&40:*int32)int32")
	if err != nil || call.Args[0].Pointee == nil || *call.Args[0].Pointee != abi.Int32(40) {
		t.Fatalf("scalar pointer: %+v %v", call, err)
	}
}

func TestSmallScalarLiterals(t *testing.T) {
	for _, tc := range []struct {
		kind abi.Type
		text string
	}{
		{abi.I8, "-128"}, {abi.U8, "255"}, {abi.I16, "-32768"}, {abi.U16, "65535"}, {abi.Bool, "true"},
	} {
		v, err := ParseValue(tc.kind, tc.text)
		if err != nil {
			t.Fatal(err)
		}
		printed, err := FormatValue(v)
		if err != nil || printed != tc.text {
			t.Fatalf("%s: %s %v", tc.text, printed, err)
		}
	}
	for _, tc := range []struct {
		kind abi.Type
		text string
	}{
		{abi.I8, "128"}, {abi.U8, "256"}, {abi.I16, "32768"}, {abi.U16, "65536"}, {abi.Bool, "1"},
	} {
		if _, err := ParseValue(tc.kind, tc.text); err == nil {
			t.Fatalf("accepted %s", tc.text)
		}
	}
}

func TestFormatMalformedAggregate(t *testing.T) {
	malformed := abi.Value{Type: abi.Struct, Aggregate: &abi.Aggregate{Type: abi.TypeDesc{Type: abi.Struct}, Fields: []abi.Value{abi.Int32(1)}}}
	if _, err := FormatValue(malformed); err == nil {
		t.Fatal("accepted malformed struct")
	}
	cycle := abi.Value{Type: abi.Pointer}
	cycle.Pointee = &cycle
	if _, err := FormatValue(cycle); err == nil {
		t.Fatal("accepted unbounded pointee cycle")
	}
}

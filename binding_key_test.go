package dylib

import (
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestBindingKeyCanonicalMetadata(t *testing.T) {
	base := abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.Pointer}}
	key, err := bindingKey(base)
	if err != nil {
		t.Fatal(err)
	}
	result := abi.TypeDesc{Type: abi.I32, Fields: []abi.Field{}}
	explicit := base.Clone()
	explicit.ResultType = &result
	explicit.ArgTypes = []abi.TypeDesc{{Type: abi.I32}, {Type: abi.Pointer, Fields: []abi.Field{}}}
	other, err := bindingKey(explicit)
	if err != nil || other != key {
		t.Fatalf("equivalent descriptor spelling: %v", err)
	}
	empty, err := bindingKey(abi.Signature{Result: abi.Void})
	other, err2 := bindingKey(abi.Signature{Result: abi.Void, Args: []abi.Type{}, ArgTypes: []abi.TypeDesc{}})
	if err != nil || err2 != nil || empty != other {
		t.Fatal("empty slices changed the signature key")
	}
}

func TestBindingKeyPreservesLogicalTypes(t *testing.T) {
	seen := make(map[string]bool)
	signatures := []abi.Signature{
		{Result: abi.F64, Args: []abi.Type{abi.I32, abi.F32}, Variadic: true, FixedArgs: 1},
		{Result: abi.F64, Args: []abi.Type{abi.I32, abi.F64}, Variadic: true, FixedArgs: 1},
		{Result: abi.F64, Args: []abi.Type{abi.I32, abi.F64}, Variadic: true, FixedArgs: 2},
		{Result: abi.F64, Args: []abi.Type{abi.I32, abi.F64}, Variadic: true, FixedArgs: 1, Convention: abi.CDecl},
		pointerArraySignature(2), pointerArraySignature(3),
		{Result: abi.Pointer, Args: []abi.Type{abi.Pointer}},
	}
	// Length prefixes preserve delimiters, empty strings and NULs in names.
	for _, names := range [][2]string{{"a", "bc"}, {"ab", "c"}, {"", "abc"}, {"a\x00", "bc"}, {"a", "\x00bc"}} {
		desc := abi.TypeDesc{Type: abi.Struct, Fields: []abi.Field{{Name: names[0], Type: abi.TypeDesc{Type: abi.I32}}, {Name: names[1], Type: abi.TypeDesc{Type: abi.I32}}}}
		signatures = append(signatures, abi.Signature{Result: abi.Struct, ResultType: &desc})
	}
	for _, sig := range signatures {
		key, err := bindingKey(sig)
		if err != nil {
			t.Fatal(err)
		}
		if seen[key] {
			t.Fatal("different logical signatures collided")
		}
		seen[key] = true
	}
	// Scalar-only encoding must not hide fields, descriptors with wrong types,
	// non-variadic boundaries or recursive/invalid pointer descriptions.
	invalid := []abi.Signature{
		{Result: abi.I32, ResultType: &abi.TypeDesc{Type: abi.I32, Fields: []abi.Field{{Type: abi.TypeDesc{Type: abi.I32}}}}},
		{Result: abi.I32, Args: []abi.Type{abi.I32}, ArgTypes: []abi.TypeDesc{{Type: abi.U32}}},
		{Result: abi.I32, FixedArgs: 1},
	}
	recursive := abi.TypeDesc{Type: abi.Pointer}
	recursive.Elem = &recursive
	invalid = append(invalid, abi.Signature{Result: abi.Pointer, ResultType: &recursive})
	for _, sig := range invalid {
		if _, err := bindingKey(sig); err == nil {
			t.Fatal("invalid metadata encoded as a usable key")
		}
	}
}

func TestNativeIndexedBindingCache(t *testing.T) {
	s := nativeABILibrary(t, "testdata/many_arguments.c", "object")
	if _, err := s.Bind("missing_indexed_symbol", pointerArraySignature(257)); err == nil || len(s.plans) != 0 {
		t.Fatal("failed symbol resolution cached a newly prepared plan")
	}
	functions := make([]*Function, 256)
	for i := range functions {
		f, err := s.Bind("identity_pointer", pointerArraySignature(i+1))
		if err != nil {
			t.Fatal(err)
		}
		functions[i] = f
	}
	for i := len(functions) - 1; i >= 0; i-- {
		sig := pointerArraySignature(i + 1)
		again, err := s.Bind("identity_pointer", sig)
		if err != nil || again.plan != functions[i].plan {
			t.Fatalf("indexed shape %d: %v", i, err)
		}
		if got, err := again.Call(abi.Ptr(0x1234)); err != nil || got != abi.Ptr(0x1234) {
			t.Fatalf("indexed pointer call: %+v, %v", got, err)
		}
		// Mutating the caller's descriptor cannot replace a cached snapshot.
		sig.ArgTypes[0].Elem.Len = 0
		if _, err := s.Bind("identity_pointer", sig); err == nil || len(s.plans) != len(functions) {
			t.Fatal("invalid metadata reused or changed the index")
		}
	}
	if err := s.Close(); err != nil || len(s.plans) != 0 {
		t.Fatalf("index cleanup: %v", err)
	}
}

func TestNativeBindingCanonicalScalarSpelling(t *testing.T) {
	s := nativeABILibrary(t, "testdata/many_arguments.c", "object")
	sig := abi.Signature{Result: abi.I32, Args: []abi.Type{abi.I32, abi.I32}}
	first, err := s.Bind("sum_two", sig)
	if err != nil {
		t.Fatal(err)
	}
	result := abi.TypeDesc{Type: abi.I32}
	sig.ResultType = &result
	sig.ArgTypes = []abi.TypeDesc{{Type: abi.I32}, {Type: abi.I32}}
	second, err := s.Bind("sub_two", sig)
	if err != nil || second.plan != first.plan || len(s.plans) != 1 {
		t.Fatalf("canonical scalar plan: %v", err)
	}
	for _, entry := range []struct {
		fn *Function
		a  int32
	}{{first, 20}, {second, 64}} {
		if got, err := entry.fn.Call(abi.Int32(entry.a), abi.Int32(22)); err != nil || got != abi.Int32(42) {
			t.Fatalf("canonical binding result: %+v, %v", got, err)
		}
	}
}

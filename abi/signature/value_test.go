package signature

import (
	"strconv"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestScalarLiterals(t *testing.T) {
	for _, tc := range []struct {
		typeID abi.Type
		text   string
		value  abi.Value
		print  string
	}{
		{abi.I32, "-2147483648", abi.Int32(-2147483648), "-2147483648"},
		{abi.U32, "0xffff_ffff", abi.Uint32(0xffffffff), "4294967295"},
		{abi.I64, "-9223372036854775808", abi.Int64(-9223372036854775808), "-9223372036854775808"},
		{abi.U64, "18446744073709551615", abi.Uint64(^uint64(0)), "18446744073709551615"},
		{abi.F32, "20.5", abi.Float32(20.5), "20.5"},
		{abi.F64, "0x1.5p+5", abi.Float64(42), "42"},
		{abi.Pointer, "nil", abi.Ptr(0), "0x0"},
		{abi.Pointer, "0x1234", abi.Ptr(0x1234), "0x1234"},
	} {
		v, err := ParseValue(tc.typeID, tc.text)
		if err != nil || v != tc.value {
			t.Fatalf("%q: %+v, %v; want %+v", tc.text, v, err, tc.value)
		}
		printed, err := FormatValue(v)
		if err != nil || printed != tc.print {
			t.Fatalf("%q: format %q, %v; want %q", tc.text, printed, err, tc.print)
		}
	}
	if text, err := FormatValue(abi.Value{Type: abi.Void}); text != "void" || err != nil {
		t.Fatalf("void: %q, %v", text, err)
	}
	_, err := ParseValue(abi.Pointer, "0x100000000")
	if (err != nil) != (strconv.IntSize == 32) {
		t.Fatalf("pointer range on %d-bit host: %v", strconv.IntSize, err)
	}
}

func TestRejectedLiterals(t *testing.T) {
	for _, tc := range []struct {
		typeID abi.Type
		text   string
	}{
		{abi.I32, "2147483648"}, {abi.I32, "-2147483649"},
		{abi.U32, "4294967296"}, {abi.U32, "-1"},
		{abi.I64, "9223372036854775808"}, {abi.U64, "18446744073709551616"},
		{abi.F32, "1e100"}, {abi.F64, "1e999"}, {abi.I32, "1+2"},
		{abi.I32, ""}, {abi.Pointer, "-1"}, {abi.Void, "0"}, {abi.Type(99), "0"},
	} {
		if v, err := ParseValue(tc.typeID, tc.text); err == nil {
			t.Fatalf("accepted type %d value %q as %+v", tc.typeID, tc.text, v)
		}
	}
	if _, err := FormatValue(abi.Value{Type: 99}); err == nil {
		t.Fatal("accepted unknown result type")
	}
}

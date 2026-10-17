//go:build cgo && (linux || darwin || windows)

package overalignedbits

import (
	"reflect"
	"testing"
)

func checkUnnamedPrefixCalls(t *testing.T, b *Bindings) {
	t.Helper()
	check := func(t *testing.T, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, %v; want %+v", got, err, want)
		}
	}
	byteValue := BindingsRecord23{Byte_tag: -127, Byte_real: 20.5}
	byteWant := BindingsRecord23{Byte_tag: -126, Byte_real: 22}
	bitValue := BindingsRecord24{Bit_flags: 7, Bit_real: 20.5}
	bitWant := BindingsRecord24{Bit_flags: 4, Bit_real: 22}
	t.Run("byte", func(t *testing.T) {
		got, err := b.Echo_byte_prefix(byteValue)
		check(t, got, byteWant, err)
	})
	t.Run("bitfield", func(t *testing.T) {
		got, err := b.Echo_bit_prefix(bitValue)
		check(t, got, bitWant, err)
	})
	t.Run("floating", func(t *testing.T) {
		got, err := b.Echo_float_prefix(BindingsRecord25{Float_first: 20.5, Float_tag: -127, Float_second: 19.5})
		check(t, got, BindingsRecord25{Float_first: 22, Float_tag: -126, Float_second: 22}, err)
	})
	t.Run("multiple", func(t *testing.T) {
		got, err := b.Echo_multi_prefix(BindingsRecord26{Multi_tag: -127, Multi_code: -32767, Multi_real: 20.5})
		check(t, got, BindingsRecord26{Multi_tag: -126, Multi_code: -32768, Multi_real: 22}, err)
	})
	t.Run("typedef", func(t *testing.T) {
		got, err := b.Echo_typedef_prefix(BindingsRecord27{Typedef_tag: -127, Typedef_real: 20.5})
		check(t, got, BindingsRecord27{Typedef_tag: -126, Typedef_real: 22}, err)
	})
	t.Run("nested", func(t *testing.T) {
		got, err := b.Echo_prefix_envelope(BindingsRecord28{Prefix_tag: -123456, Prefix_items: [2]BindingsRecord23{byteValue, byteValue}, Prefix_bits: bitValue})
		check(t, got, BindingsRecord28{Prefix_tag: -123456, Prefix_items: [2]BindingsRecord23{byteWant, byteWant}, Prefix_bits: bitWant}, err)
	})
	t.Run("variadic", func(t *testing.T) {
		got, err := b.Prefix_variable(bitValue, 3, 1.5)
		check(t, got, bitWant, err)
	})
	t.Run("overflow", func(t *testing.T) {
		got, err := b.Prefix_overflow(1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 7, 8, bitValue, 8)
		check(t, got, float64(92.5), err)
	})
	t.Run("factory", func(t *testing.T) {
		entry, err := b.Prefix_factory()
		if err != nil || entry == nil {
			t.Fatal("native function pointer", entry, err)
		}
		got, err := b.Forward_prefix(entry, bitValue)
		check(t, got, bitWant, err)
		got, err = b.Forward_prefix(nil, bitValue)
		check(t, got, bitValue, err)
	})
	t.Run("pointer", func(t *testing.T) {
		address, err := b.Prefix_pointer(bitValue)
		if err != nil || address == nil {
			t.Fatal("native struct pointer", address, err)
		}
		got, err := b.Read_prefix(address)
		check(t, got, bitValue, err)
		got, err = b.Read_prefix(nil)
		check(t, got, BindingsRecord24{}, err)
	})
}

//go:build cgo && (linux || darwin || windows)

package overalignedbits

import (
	"reflect"
	"testing"
)

func checkPackedRootCalls(t *testing.T, b *Bindings) {
	t.Helper()
	check := func(t *testing.T, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, %v; want %+v", got, err, want)
		}
	}
	byteValue := BindingsRecord29{Root_byte_tag: 20, Root_byte_real: 20.5}
	byteWant := BindingsRecord29{Root_byte_tag: 21, Root_byte_real: 22}
	bitValue := BindingsRecord31{Root_named_flags: 7, Root_named_tag: 20, Root_named_real: 20.5}
	bitWant := BindingsRecord31{Root_named_flags: 4, Root_named_tag: 21, Root_named_real: 22}
	floatValue := BindingsRecord32{Root_floating_head: 20.5, Root_floating_tag: 20, Root_floating_real: 19.5}
	floatWant := BindingsRecord32{Root_floating_head: 22, Root_floating_tag: 21, Root_floating_real: 22}
	envelope := BindingsRecord34{Root_items: [2]BindingsRecord29{byteValue, byteValue}, Root_bits: bitValue, Root_floats: floatValue}
	envelopeWant := BindingsRecord34{Root_items: [2]BindingsRecord29{byteWant, byteWant}, Root_bits: bitWant, Root_floats: floatWant}
	t.Run("byte", func(t *testing.T) {
		got, err := b.Echo_root_byte(byteValue)
		check(t, got, byteWant, err)
	})
	t.Run("leading", func(t *testing.T) {
		got, err := b.Echo_root_leading(BindingsRecord30{Root_leading_tag: 20, Root_leading_real: 20.5})
		check(t, got, BindingsRecord30{Root_leading_tag: 21, Root_leading_real: 22}, err)
	})
	t.Run("named", func(t *testing.T) {
		got, err := b.Echo_root_named(bitValue)
		check(t, got, bitWant, err)
	})
	t.Run("floating", func(t *testing.T) {
		got, err := b.Echo_root_floating(floatValue)
		check(t, got, floatWant, err)
	})
	t.Run("aligned", func(t *testing.T) {
		got, err := b.Echo_root_aligned(BindingsRecord33{Root_aligned_tag: 20, Root_aligned_real: 20.5})
		check(t, got, BindingsRecord33{Root_aligned_tag: 21, Root_aligned_real: 22}, err)
	})
	t.Run("nested", func(t *testing.T) {
		got, err := b.Echo_root_envelope(envelope)
		check(t, got, envelopeWant, err)
	})
	t.Run("variadic", func(t *testing.T) {
		got, err := b.Root_variable(byteValue, -1, 1.5)
		check(t, got, BindingsRecord29{Root_byte_tag: 19, Root_byte_real: 22}, err)
	})
	t.Run("overflow", func(t *testing.T) {
		got, err := b.Root_overflow(1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 7, 8, byteValue, 8)
		check(t, got, float64(105.5), err)
	})
	t.Run("factory", func(t *testing.T) {
		entry, err := b.Root_factory()
		if err != nil || entry == nil {
			t.Fatal("native function pointer", entry, err)
		}
		got, err := b.Forward_root(entry, byteValue)
		check(t, got, byteWant, err)
		got, err = b.Forward_root(nil, byteValue)
		check(t, got, byteValue, err)
	})
	t.Run("pointer", func(t *testing.T) {
		address, err := b.Root_pointer(envelope)
		if err != nil || address == nil {
			t.Fatal("native packed record pointer", address, err)
		}
		got, err := b.Read_root(address)
		check(t, got, envelope, err)
		got, err = b.Read_root(nil)
		check(t, got, BindingsRecord34{}, err)
	})
}

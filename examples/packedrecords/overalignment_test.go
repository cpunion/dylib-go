//go:build cgo && (linux || darwin || windows)

package packedrecords

import (
	"reflect"
	"testing"
)

// The caller holds a module lease throughout native address/entry use.
func checkOverAlignedCalls(t *testing.T, b *Bindings) {
	t.Helper()
	check := func(name string, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %+v, %v; want %+v", name, got, err, want)
		}
	}
	address, err := b.Over_pointer()
	if err != nil || address == nil {
		t.Fatal("over-aligned native storage", address, err)
	}
	p := BindingsRecord10{Over_tag: -128, Over_count: -32767, Over_samples: [2]float64{20.5, 19.5}, Over_truth: true, Over_context: address}
	want := BindingsRecord10{Over_tag: -127, Over_count: -32765, Over_samples: [2]float64{22, 22}, Over_context: address}
	got, err := b.Echo_over(p)
	check("pragma member over-alignment", got, want, err)
	entry, err := b.Over_factory()
	if err != nil || entry == nil {
		t.Fatal("over-aligned factory", entry, err)
	}
	got, err = b.Apply_over(entry, p)
	check("over-aligned native entry", got, want, err)
	slotWant := want
	slotWant.Over_tag++
	got, err = b.Apply_over_slot(BindingsRecord18{Over_slot_tag: 1, Over_entry: entry}, p)
	check("over-aligned function-pointer member", got, slotWant, err)
	got, err = b.Apply_over_slot(BindingsRecord18{}, p)
	check("null over-aligned entry", got, p, err)
	packed := BindingsRecord11{Packed_tag: -128, Packed_count: 20, Packed_real: 20.5}
	packedWant := BindingsRecord11{Packed_tag: -127, Packed_count: 22, Packed_real: 22}
	packedGot, err := b.Echo_over_packed(packed)
	check("pragma and packed member alignment", packedGot, packedWant, err)
	maximum := BindingsRecord12{Max_tag: -128, Max_count: 20, Max_real: 20.5}
	maximumWant := BindingsRecord12{Max_tag: -127, Max_count: 22, Max_real: 22}
	maximumGot, err := b.Echo_over_max(maximum)
	check("omitted GNU alignment under pragma", maximumGot, maximumWant, err)
	typed := BindingsRecord13{Type_tag: -128, Type_count: 20, Type_real: 20.5}
	typedWant := BindingsRecord13{Type_tag: -127, Type_count: 22, Type_real: 22}
	typedGot, err := b.Echo_over_type(typed)
	check("type-based alignment under pragma", typedGot, typedWant, err)
	array := BindingsRecord14{Array_tag: -128, Array_values: [2]float64{20.5, 19.5}, Array_count: 20}
	arrayWant := BindingsRecord14{Array_tag: -127, Array_values: [2]float64{22, 22}, Array_count: 22}
	arrayGot, err := b.Echo_over_array(array)
	check("over-aligned array field", arrayGot, arrayWant, err)
	aligned := BindingsRecord7{Aligned_tag: -128, Aligned_count: 20, Aligned_value: 20.5}
	alignedWant := BindingsRecord7{Aligned_tag: -127, Aligned_count: 22, Aligned_value: 22}
	envelope := BindingsRecord15{
		Nested_tag: -128, Nested_count: 20,
		Nested_items:   [2][2]BindingsRecord10{{p, p}, {p, p}},
		Nested_packed:  [2]BindingsRecord11{packed, packed},
		Nested_max:     [2]BindingsRecord12{maximum, maximum},
		Nested_type:    [2]BindingsRecord13{typed, typed},
		Nested_array:   [2]BindingsRecord14{array, array},
		Nested_aligned: [2]BindingsRecord7{aligned, aligned},
	}
	envelopeWant := BindingsRecord15{
		Nested_tag: -127, Nested_count: 22,
		Nested_items:   [2][2]BindingsRecord10{{want, want}, {want, want}},
		Nested_packed:  [2]BindingsRecord11{packedWant, packedWant},
		Nested_max:     [2]BindingsRecord12{maximumWant, maximumWant},
		Nested_type:    [2]BindingsRecord13{typedWant, typedWant},
		Nested_array:   [2]BindingsRecord14{arrayWant, arrayWant},
		Nested_aligned: [2]BindingsRecord7{alignedWant, alignedWant},
	}
	nested, err := b.Echo_over_envelope(envelope)
	check("nested over-aligned values and arrays", nested, envelopeWant, err)
	floating, err := b.Echo_over_float(BindingsRecord16{Over_left: 20.5, Over_right: 19.5})
	check("over-aligned homogeneous float aggregate", floating, BindingsRecord16{Over_left: 22, Over_right: 22}, err)
	floatNested, err := b.Echo_over_float_nested(BindingsRecord19{Over_floats: [1]BindingsRecord16{{Over_left: 20.5, Over_right: 19.5}}, Over_float_tail: 18.5})
	check("nested over-aligned homogeneous float aggregate", floatNested, BindingsRecord19{Over_floats: [1]BindingsRecord16{{Over_left: 22, Over_right: 22}}, Over_float_tail: 22}, err)
	outer, err := b.Echo_over_outer(BindingsRecord17{Over_payload: envelope, Over_tail: 19.5})
	check("natural outer record after pragma pop", outer, BindingsRecord17{Over_payload: envelopeWant, Over_tail: 22}, err)
	got, err = b.Over_variable(p, -8, 1.5)
	variableWant := p
	variableWant.Over_count -= 8
	variableWant.Over_samples[0] += 1.5
	check("over-aligned variadic prefix and result", got, variableWant, err)
	plain := p
	plain.Over_tag, plain.Over_truth, plain.Over_context = 125, false, nil
	plainWant := plain
	plainWant.Over_tag++
	plainWant.Over_count += 2
	plainWant.Over_samples = [2]float64{22, 22}
	plainWant.Over_truth = true
	got, err = b.Echo_over(plain)
	check("over-aligned false and null values", got, plainWant, err)
	changed, err := b.Mutate_over(address)
	check("over-aligned pointer identity", changed, address, err)
	sum, err := b.Sum_over(address)
	check("over-aligned pointer mutation", sum, float64(44), err)
	changed, err = b.Mutate_over(nil)
	if err != nil || changed != nil {
		t.Fatal("null over-aligned pointer", changed, err)
	}
	sum, err = b.Sum_over(nil)
	check("null over-aligned pointer sentinel", sum, float64(-1), err)
}

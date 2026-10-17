//go:build cgo && (linux || darwin || windows)

package overalignedbits

import (
	"reflect"
	"testing"
)

func checkPaddingClassCalls(t *testing.T, b *Bindings) {
	t.Helper()
	check := func(t *testing.T, got, want any, err error) {
		t.Helper()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, %v; want %+v", got, err, want)
		}
	}
	t.Run("wide", func(t *testing.T) {
		got, err := b.Echo_class_wide(BindingsRecord35{Class_wide_real: 20.5, Class_wide_tag: 20})
		check(t, got, BindingsRecord35{Class_wide_real: 22, Class_wide_tag: 21}, err)
	})
	t.Run("nested", func(t *testing.T) {
		got, err := b.Echo_class_nested(BindingsRecord37{Class_integer: BindingsRecord36{Class_inner_integer: 20}, Class_nested_real: 20.5})
		check(t, got, BindingsRecord37{Class_integer: BindingsRecord36{Class_inner_integer: 21}, Class_nested_real: 22}, err)
	})
	t.Run("array", func(t *testing.T) {
		got, err := b.Echo_class_array(BindingsRecord38{Class_items: [1]int16{20}, Class_array_real: 20.5})
		check(t, got, BindingsRecord38{Class_items: [1]int16{21}, Class_array_real: 22}, err)
	})
}

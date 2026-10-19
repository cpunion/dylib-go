//go:build cgo

package nominalrecords

import "testing"

func checkUnnamed(t *testing.T, b *Bindings) {
	t.Helper()
	t.Run("unnamed_wide", func(t *testing.T) {
		got, err := b.Echo_wide(BindingsRecord4{X: 40.5, Y: 20})
		if err != nil || got != (BindingsRecord4{X: 42, Y: 21}) {
			t.Fatal(got, err)
		}
	})
	t.Run("unnamed_small", func(t *testing.T) {
		got, err := b.Echo_small(BindingsRecord5{X: 40.5, Y: 20})
		if err != nil || got != (BindingsRecord5{X: 42, Y: 21}) {
			t.Fatal(got, err)
		}
	})
}

//go:build cgo && (linux || darwin || windows)

package recordcallbacks

import "testing"

func TestNativeCallbackArrayRecords(t *testing.T) {
	withFixtures(t, func(t *testing.T, path string) {
		s, b := open(t, path)
		owner, err := s.Resolve("callback_array_factory")
		if err != nil {
			t.Fatal(err)
		}
		err = owner.WithAddress(func(uintptr) error {
			r, err := b.Callback_array_factory()
			if err != nil {
				return err
			}
			r, err = b.Callback_array_echo(r)
			if err != nil {
				return err
			}
			for _, call := range []struct{ row, column, a, b, want int32 }{{0, 0, 20, 22, 42}, {0, 1, 64, 22, 42}, {1, 0, 20, 22, 42}, {1, 1, 20, 22, -1}, {2, 0, 20, 22, -1}} {
				got, err := b.Callback_array_apply(r, call.row, call.column, call.a, call.b)
				if err != nil || got != call.want {
					t.Fatal(call, got, err)
				}
			}
			// Replacing one entry preserves all other elements and their order.
			r.Entries[1][1] = r.Entries[0][1]
			if err := b.Callback_array_store(r); err != nil {
				return err
			}
			if got, err := b.Callback_array_run(1, 1, 64, 22); err != nil || got != 42 {
				t.Fatal(got, err)
			}
			if err := b.Callback_array_clear(); err != nil {
				return err
			}
			if got, err := b.Callback_array_run(1, 1, 64, 22); err != nil || got != -1 {
				t.Fatal("clear", got, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

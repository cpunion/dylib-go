//go:build !llgo && cgo && (linux || darwin || windows)

package recordcallbacks

import (
	"github.com/cpunion/dylib-go/compiler/clang"
	"testing"
)

const backend = "cgo"

func generate(h *clang.Header) ([]byte, error) { return h.CgoSource("recordcallbacks", "CBindings") }
func checkPointer(t *testing.T, b *Bindings) error {
	ptr, err := b.Callbacks_slot()
	if err != nil {
		return err
	}
	got, err := b.Callbacks_pointer(ptr, 20, 22)
	if err != nil || got != 42 {
		t.Fatal(got, err)
	}
	return nil
}

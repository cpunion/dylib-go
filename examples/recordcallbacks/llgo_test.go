//go:build llgo && cgo && (linux || darwin || windows)

package recordcallbacks

import (
	"github.com/cpunion/dylib-go/compiler/clang"
	"testing"
	"unsafe"
)

const backend = "llgo"

func generate(h *clang.Header) ([]byte, error) { return h.LLGoSource("recordcallbacks", "Bindings") }
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

//llgo:type C
type adder func(int32, int32) int32

func goAdder(a, b int32) int32 { return a + b + 1 }

func TestLLGoCallbackField(t *testing.T) {
	withFixtures(t, func(t *testing.T, path string) {
		_, b := open(t, path)
		entry := adder(goAdder)
		address := *(*unsafe.Pointer)(unsafe.Pointer(&entry))
		got, err := b.Callbacks_apply(BindingsRecord0{Add: address}, 20, 21)
		if err != nil || got != 42 {
			t.Fatal(got, err)
		}
	})
}

func TestLLGoCgoCallbackRecords(t *testing.T) {
	withFixtures(t, func(t *testing.T, path string) {
		s, _ := open(t, path)
		b, err := NewCBindings(s)
		if err != nil {
			t.Fatal(err)
		}
		r, err := b.Callbacks_factory(1)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := b.Callbacks_apply(r, 20, 21); err != nil || got != 42 {
			t.Fatal(got, err)
		}
		nested, err := b.Callbacks_nested(CBindingsRecord1{Items: [3]CBindingsRecord0{r, r, r}})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := b.Callbacks_nested_apply(nested, 20, 21); err != nil || got != 42 {
			t.Fatal(got, err)
		}
	})
}

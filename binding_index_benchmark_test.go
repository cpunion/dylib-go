package dylib

import (
	"strconv"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func pointerArraySignature(length int) abi.Signature {
	elem := abi.TypeDesc{Type: abi.I32}
	array := abi.TypeDesc{Type: abi.Array, Elem: &elem, Len: length}
	return abi.Signature{Result: abi.Pointer, Args: []abi.Type{abi.Pointer}, ArgTypes: []abi.TypeDesc{{Type: abi.Pointer, Elem: &array}}}
}

// Each signature is a valid C pointer shape; native plans are prepared before
// timing. Rebind the last shape to expose lookup cost as the cache grows.
func BenchmarkNativeBindingCacheSize(b *testing.B) {
	for _, size := range []int{1, 16, 256, 4096} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			s := nativeABILibrary(b, "testdata/many_arguments.c", "object")
			var last *Function
			for i := 1; i <= size; i++ {
				f, err := s.Bind("identity_pointer", pointerArraySignature(i))
				if err != nil {
					b.Fatal(err)
				}
				last = f
			}
			signature := pointerArraySignature(size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f, err := s.Bind("identity_pointer", signature)
				if err != nil || f.plan != last.plan {
					b.Fatalf("cache lookup: %v", err)
				}
			}
		})
	}
}

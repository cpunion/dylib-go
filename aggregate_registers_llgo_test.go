//go:build llgo && cgo && (amd64 || arm64) && (linux || darwin || windows)

package dylib

import (
	"testing"
	"unsafe"
)

type registerFloatPair struct{ A, B float32 }
type registerIntegerPair struct{ A, B int64 }
type registerMixedPair struct {
	A float64
	B int64
}
type registerFloatTriple struct {
	A    float32
	Rest [2]float32
}
type registerDoubleQuad struct{ Values [4]float64 }
type registerSmall struct{ A, B int32 }
type registerLarge struct{ A, B, C int64 }

//llgo:cdecl
type exhaustedFloatEntry func(float64, float64, float64, float64, float64, float64, float64, float64, registerFloatPair, float64) float64

//llgo:cdecl
type partialFloatEntry func(float64, float64, float64, float64, float64, float64, float64, registerFloatPair, float64) float64

//llgo:cdecl
type partialIntegerEntry func(int64, int64, int64, int64, int64, registerIntegerPair, int64) int64

//llgo:cdecl
type rollbackIntegerEntry func(int64, int64, int64, int64, int64, int64, registerMixedPair, float64) float64

//llgo:cdecl
type rollbackFloatEntry func(float64, float64, float64, float64, float64, float64, float64, float64, registerMixedPair, int64) float64

//llgo:cdecl
type stackedTripleEntry func(float64, float64, float64, float64, float64, float64, float64, float64, float32, registerFloatTriple, float64) float64

//llgo:cdecl
type stackedQuadEntry func(float64, float64, float64, float64, float64, float64, float64, float64, registerDoubleQuad, float64) float64

//llgo:cdecl
type hiddenResultEntry func(int64, int64, int64, int64, int64, registerSmall, int64) registerLarge

func TestNativeDirectAggregateRegisters(t *testing.T) {
	withAggregateRegisters(t, func(t *testing.T, s *Session) {
		for _, tc := range []struct {
			name string
			call func(uintptr) float64
		}{
			{"exhausted_float", func(p uintptr) float64 {
				f := *(*exhaustedFloatEntry)(unsafe.Pointer(&p))
				return f(1, 2, 3, 4, 5, 6, 7, 8, registerFloatPair{1, 2}, 3)
			}},
			{"partial_float", func(p uintptr) float64 {
				f := *(*partialFloatEntry)(unsafe.Pointer(&p))
				return f(1, 2, 3, 4, 5, 6, 7, registerFloatPair{6, 7}, 1)
			}},
			{"partial_integer", func(p uintptr) float64 {
				f := *(*partialIntegerEntry)(unsafe.Pointer(&p))
				return float64(f(1, 2, 3, 4, 5, registerIntegerPair{6, 7}, 14))
			}},
			{"rollback_integer", func(p uintptr) float64 {
				f := *(*rollbackIntegerEntry)(unsafe.Pointer(&p))
				return f(1, 2, 3, 4, 5, 6, registerMixedPair{7, 8}, 6)
			}},
			{"rollback_float", func(p uintptr) float64 {
				f := *(*rollbackFloatEntry)(unsafe.Pointer(&p))
				return f(1, 2, 3, 4, 5, 6, 7, 8, registerMixedPair{1, 2}, 3)
			}},
			{"stacked_triple", func(p uintptr) float64 {
				f := *(*stackedTripleEntry)(unsafe.Pointer(&p))
				return f(1, 2, 3, 4, 5, 6, 7, 8, 1, registerFloatTriple{1, [2]float32{1, 1}}, 2)
			}},
			{"stacked_quad", func(p uintptr) float64 {
				f := *(*stackedQuadEntry)(unsafe.Pointer(&p))
				return f(1, 2, 3, 4, 5, 6, 7, 8, registerDoubleQuad{[4]float64{1, 1, 1, 1}}, 2)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				symbol, err := s.Resolve(tc.name)
				if err != nil {
					t.Fatal(err)
				}
				if err := symbol.WithAddress(func(p uintptr) error {
					if got := tc.call(p); got != 42 {
						t.Fatalf("got %v, want 42", got)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			})
		}
		t.Run("hidden_result", func(t *testing.T) {
			symbol, err := s.Resolve("hidden_result")
			if err != nil {
				t.Fatal(err)
			}
			if err := symbol.WithAddress(func(p uintptr) error {
				f := *(*hiddenResultEntry)(unsafe.Pointer(&p))
				if got := f(1, 2, 3, 4, 5, registerSmall{6, 7}, 14); got != (registerLarge{15, 13, 14}) {
					t.Fatal(got)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	})
}

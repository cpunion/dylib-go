//go:build llgo && cgo && (linux || darwin || windows)

package main

import (
	"testing"

	dylib "github.com/ramblelabs/dylib-go"
)

func checkDirectNarrowCalls(t *testing.T, session *dylib.Session, b *Bindings) {
	t.Helper()
	check := func(name string, got, want int32, err error) {
		t.Helper()
		if err != nil || got != want {
			t.Fatalf("%s: %d, %v; want %d", name, got, err, want)
		}
	}
	// Match the C arithmetic with wide results so return truncation cannot
	// conceal an incorrectly extended negative argument.
	wide := func(a int8, b uint8, c int16, d uint16, flag bool) int32 {
		value := 3*int32(a) + int32(b) + 5*int32(c) + int32(d)
		if flag {
			value += 101
		}
		return value
	}
	factory, err := session.Resolve("narrow_factory")
	if err != nil {
		t.Fatal(err)
	}
	// Factory calls guard their own invocation; retain the producer for every
	// subsequent native entry invocation and forwarding call as well.
	err = factory.WithAddress(func(_ uintptr) error {
		entry, err := b.Narrow_factory()
		if err != nil || entry == nil {
			t.Fatal("narrow native factory:", err)
		}
		variadic, err := b.Narrow_variadic_factory()
		if err != nil || variadic == nil {
			t.Fatal("narrow variadic factory:", err)
		}
		// This noncapturing C entry reenters the registered calling Go thread.
		// Use a wide result; narrow Go-to-C results need a separate adapter.
		local := BindingsCallback5(func(a int8, b uint8, c int16, d uint16, flag bool) int32 {
			result := 3*int32(a) + int32(b) + 5*int32(c) + int32(d)
			if flag {
				result += 101
			}
			return result
		})
		for _, input := range []struct {
			name string
			a    int8
			b    uint8
			c    int16
			d    uint16
		}{
			{"minimum", -128, 255, -32768, 65535},
			{"negative", -8, 255, -300, 65535},
			{"minus-one", -1, 128, -1, 32768},
			{"zero", 0, 0, 0, 0},
			{"maximum", 127, 255, 32767, 65535},
		} {
			for _, flag := range []bool{false, true} {
				a, u8, c, u16 := input.a, input.b, input.c, input.d
				want := wide(a, u8, c, u16, flag)
				got, err := b.Narrow(a, u8, c, u16, flag)
				check(input.name+" method", got, want, err)
				check(input.name+" native pointer", entry(a, u8, c, u16, flag), want, nil)
				got, err = b.Apply_narrow(entry, a, u8, c, u16, flag)
				check(input.name+" C forwarding", got, want, err)
				got, err = b.Apply_narrow(local, a, u8, c, u16, flag)
				check(input.name+" C to Go arguments", got, want, err)
				stack, err := b.Narrow_stack(1, 2, 3, 4, 5, 6, 7, 8, a, u8, c, u16, flag)
				check(input.name+" stack/sret", stack.Values[0][0], want, err)
				check(input.name+" leading stack arguments", stack.Values[0][1], 204, nil)
				got, err = b.Narrow_variable(a, u8, c, u16, flag, 2, -7, 12)
				check(input.name+" fixed variadic prefix", got, want+5, err)
				check(input.name+" variadic native pointer", variadic(a, u8, c, u16, flag, 2, int32(-7), float64(12)), want+5, nil)
				check(input.name+" empty variadic native tail", variadic(a, u8, c, u16, flag, 0), want, nil)
			}
		}
		got, err := b.Forward_narrow_variadic(variadic)
		check("C variadic pointer forwarding", got, wide(-8, 255, -300, 65535, true)+5, err)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

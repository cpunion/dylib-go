package abi

import (
	"runtime"
	"testing"
)

func TestAMD64ConventionValidation(t *testing.T) {
	for _, convention := range []Convention{Win64, SysV64} {
		s := Signature{Convention: convention, Result: I32, Args: []Type{I32, I32}}
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
		variable := s
		variable.Variadic = true
		variable.FixedArgs = 1
		if err := variable.Validate(); err == nil {
			t.Fatal("non-default variadic contract accepted")
		}
		unsupported := runtime.GOARCH != "amd64" || convention == SysV64 && runtime.GOOS == "windows"
		if !Available() || unsupported {
			if p, err := Prepare(s); err == nil {
				p.Close()
				t.Fatal("unavailable ABI accepted", convention)
			}
			if cb, err := NewCallback(s, func([]Value) (Value, error) { return Int32(0), nil }); err == nil {
				cb.Close()
				t.Fatal("unavailable closure ABI accepted", convention)
			}
		} else {
			p, err := Prepare(s)
			if err != nil {
				t.Fatal(err)
			}
			p.Close()
			want, err := LayoutOf(TypeDesc{Type: Pointer}, Default)
			if err != nil {
				t.Fatal(err)
			}
			got, err := LayoutOf(TypeDesc{Type: Pointer}, convention)
			if err != nil || got.Size != want.Size || got.Alignment != want.Alignment {
				t.Fatal("foreign ABI pointer storage:", got, err)
			}
			cb, err := NewCallback(s, func([]Value) (Value, error) { return Int32(0), nil })
			if err != nil {
				t.Fatal(err)
			}
			cb.Close()
		}
	}
}

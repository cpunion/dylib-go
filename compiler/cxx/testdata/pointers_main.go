package main

import (
	"fmt"
	"os"
	"unsafe"

	dylib "github.com/ramblelabs/dylib-go"
	"github.com/ramblelabs/dylib-go/compiler/cxx"
)

func check(status int32, err error) {
	if err != nil || status != cxx.Success {
		panic(fmt.Sprintf("C++ status %d: %v", status, err))
	}
}

func main() {
	session := dylib.New(dylib.Options{})
	defer session.Close()
	if err := session.Load(os.Args[1]); err != nil {
		panic(err)
	}
	b, err := NewBindings(session)
	if err != nil {
		panic(err)
	}
	var numbers, constant, slot, rows, text, mode, payload, opaque unsafe.Pointer
	var value int32
	out := unsafe.Pointer(&value)
	want := func(expected int32) {
		if value != expected {
			panic(fmt.Sprintf("pointer call returned %d; want %d", value, expected))
		}
	}
	check(b.Pointer_numbers(unsafe.Pointer(&numbers)))
	check(b.Pointer_readonly(unsafe.Pointer(&constant)))
	if numbers == nil || constant != numbers {
		panic("const native pointer identity")
	}
	check(b.Pointer_value(constant, out))
	want(20)
	check(b.Pointer_add(numbers, 22, out))
	want(42)
	check(b.Pointer_value(constant, out))
	want(42)
	check(b.Pointer_slot(unsafe.Pointer(&slot)))
	check(b.Pointer_readonly_slot(unsafe.Pointer(&constant)))
	if slot == nil || slot != constant {
		panic("const pointer slot identity")
	}
	check(b.Pointer_advance(slot, out))
	want(22)
	check(b.Pointer_read_slot(constant, out))
	want(22)
	check(b.Pointer_rows(unsafe.Pointer(&rows)))
	check(b.Pointer_row_sum(rows, out))
	want(42)
	check(b.Pointer_text(unsafe.Pointer(&text)))
	check(b.Pointer_parse(text, out))
	want(42)
	check(b.Pointer_mode(unsafe.Pointer(&mode)))
	check(b.Pointer_read_mode(mode, out))
	want(42)
	check(b.Pointer_payload(unsafe.Pointer(&payload)))
	check(b.Pointer_read_integer(payload, out))
	want(42)
	var floating float64
	check(b.Pointer_set_decimal(payload, 42.5, unsafe.Pointer(&floating)))
	if floating != 42.5 {
		panic("native union active-member write")
	}
	floating = 0
	check(b.Pointer_read_decimal(payload, unsafe.Pointer(&floating)))
	if floating != 42.5 {
		panic("native union active-member read")
	}
	check(b.Pointer_opaque(payload, unsafe.Pointer(&opaque)))
	if opaque != payload {
		panic("void pointer round trip")
	}
	check(b.Pointer_empty(unsafe.Pointer(&opaque)))
	if opaque != nil {
		panic("null typed pointer result")
	}
	check(b.Pointer_value(nil, out))
	want(-1)
	check(b.Pointer_opaque(nil, unsafe.Pointer(&opaque)))
	if opaque != nil {
		panic("null void pointer round trip")
	}
	fmt.Println(42)
}

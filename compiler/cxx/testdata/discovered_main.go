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
	var derived, diamond, counter, left, indirect unsafe.Pointer
	check(b.Discovered_derived_new(20, unsafe.Pointer(&derived)))
	defer func() { check(b.Discovered_derived_delete(unsafe.Pointer(&derived))) }()
	check(b.Discovered_diamond_new(20, unsafe.Pointer(&diamond)))
	defer func() { check(b.Discovered_diamond_delete(unsafe.Pointer(&diamond))) }()
	var value int32
	check(b.Discovered_derived_counter(derived, unsafe.Pointer(&counter)))
	if counter == nil || counter == derived {
		panic("missing discovered secondary-base adjustment")
	}
	check(b.Discovered_score(counter, 20, unsafe.Pointer(&value)))
	if value != 42 {
		panic("discovered secondary-base virtual call")
	}
	check(b.Discovered_diamond_counter(diamond, unsafe.Pointer(&counter)))
	check(b.Discovered_diamond_virtualleft(diamond, unsafe.Pointer(&left)))
	check(b.Discovered_virtualleft_counter(left, unsafe.Pointer(&indirect)))
	if counter == nil || counter == diamond || indirect != counter {
		panic("discovered diamond did not share its virtual base")
	}
	check(b.Discovered_score(counter, 20, unsafe.Pointer(&value)))
	if value != 42 {
		panic("discovered virtual-base dispatch")
	}
	fmt.Println(42)
}

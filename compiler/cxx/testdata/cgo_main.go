package main

import (
	"fmt"
	"os"
	"unsafe"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/compiler/cxx"
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
	// Keep every class entry alive for the entire native object's lifetime.
	factory, err := session.Resolve("counter_new")
	if err != nil {
		panic(err)
	}
	code, err := factory.Acquire()
	if err != nil {
		panic(err)
	}
	defer code.Close()
	var handle unsafe.Pointer
	slot := unsafe.Pointer(&handle)
	status, err := b.Counter_new(-1, slot)
	if err != nil || status != cxx.Exception || handle != nil {
		panic("throwing constructor escaped or published a handle")
	}
	check(b.Counter_new(20, slot))
	defer func() { check(b.Counter_delete(slot)) }()
	before := handle
	status, err = b.Counter_new(99, slot)
	if err != nil || status != cxx.InvalidArgument || handle != before {
		panic("constructor overwrote an owned handle")
	}
	var result int32
	check(b.Counter_add(handle, 22, unsafe.Pointer(&result)))
	if result != 42 {
		panic("wrong member result")
	}
	status, err = b.Counter_add(handle, -1, unsafe.Pointer(&result))
	if err != nil || status != cxx.Exception || result != 0 {
		panic("throwing member escaped or left an output")
	}
	check(b.Counter_delete(slot))
	if handle != nil {
		panic("destruction did not clear the handle")
	}
	check(b.Counter_delete(slot))
	fmt.Println(42)
}

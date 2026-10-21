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
	factory, err := session.Resolve("callback_factory")
	if err != nil {
		panic(err)
	}
	lease, err := factory.Acquire()
	if err != nil {
		panic(err)
	}
	defer lease.Close()
	var entry, owner, previous, counter unsafe.Pointer
	check(b.Callback_factory(false, unsafe.Pointer(&entry)))
	if entry == nil {
		panic("missing native function pointer")
	}
	check(b.Callback_new(entry, unsafe.Pointer(&owner)))
	defer func() { check(b.Callback_delete(unsafe.Pointer(&owner))) }()
	var value int32
	out := unsafe.Pointer(&value)
	check(b.Callback_invoke(owner, 20, 22, out))
	if value != 42 {
		panic("retained native callback")
	}
	check(b.Callback_exchange(owner, nil, unsafe.Pointer(&previous)))
	if previous != entry {
		panic("native callback return round trip")
	}
	check(b.Callback_invoke(owner, 20, 22, out))
	if value != -1 {
		panic("null function argument was not passed through")
	}
	check(b.Callback_apply(previous, 20, 22, out))
	if value != 42 {
		panic("returned callback invocation")
	}
	check(b.Object_new(20, unsafe.Pointer(&counter)))
	defer func() { check(b.Object_delete(unsafe.Pointer(&counter))) }()
	check(b.Callback_mixed(owner, 2, counter, entry, previous, out))
	if value != 42 {
		panic("multiple callbacks mixed with scalar and object arguments")
	}
	check(b.Callback_exchange(owner, entry, unsafe.Pointer(&previous)))
	if previous != nil {
		panic("null function result")
	}
	check(b.Callback_shutdown(owner, out))
	if value != 40 {
		panic("native shutdown did not invoke retained callback")
	}
	check(b.Callback_empty(unsafe.Pointer(&previous)))
	if previous != nil {
		panic("null function factory result")
	}
	previous = entry
	status, err := b.Callback_factory(true, unsafe.Pointer(&previous))
	if err != nil || status != cxx.Exception || previous != nil {
		panic("throwing factory did not clear its output")
	}
	status, err = b.Callback_exchange(nil, entry, unsafe.Pointer(&previous))
	if err != nil || status != cxx.InvalidArgument || previous != nil {
		panic("null receiver did not clear its function result")
	}
	check(b.Callback_delete(unsafe.Pointer(&owner)))
	check(b.Callback_new(nil, unsafe.Pointer(&owner)))
	check(b.Callback_invoke(owner, 20, 22, out))
	if value != -1 {
		panic("null constructor callback")
	}
	fmt.Println(42)
}

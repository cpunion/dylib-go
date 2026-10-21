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
	var function, copied, captured, clone, moving, entry unsafe.Pointer
	defer func() {
		check(b.Function_delete(unsafe.Pointer(&function)))
		check(b.Function_delete(unsafe.Pointer(&copied)))
		check(b.Captured_delete(unsafe.Pointer(&captured)))
		check(b.Captured_delete(unsafe.Pointer(&clone)))
		check(b.Move_only_delete(unsafe.Pointer(&moving)))
	}()
	var value int32
	out := unsafe.Pointer(&value)
	want := func(expected int32) {
		if value != expected {
			panic(fmt.Sprintf("callable returned %d; want %d", value, expected))
		}
	}
	check(b.Function_make(2, unsafe.Pointer(&function)))
	check(b.Function_call(function, 20, 20, out))
	want(42)
	check(b.Function_copy(function, unsafe.Pointer(&copied)))
	check(b.Function_call(function, 1, 2, out))
	want(45)
	check(b.Function_call(copied, 0, 0, out))
	want(42)
	check(b.Function_delete(unsafe.Pointer(&function)))
	check(b.Function_empty(unsafe.Pointer(&function)))
	value = 999
	status, err := b.Function_call(function, 20, 22, out)
	if err != nil || status != cxx.Exception {
		panic("empty std::function did not report bad_function_call")
	}
	want(0)
	check(b.Function_delete(unsafe.Pointer(&function)))
	check(b.Callable_entry(unsafe.Pointer(&entry)))
	check(b.Function_new(entry, unsafe.Pointer(&function)))
	check(b.Function_call(function, 20, 22, out))
	want(42)
	check(b.Captured_make(2, unsafe.Pointer(&captured)))
	check(b.Captured_call(captured, 20, 20, out))
	want(42)
	check(b.Captured_copy(captured, unsafe.Pointer(&clone)))
	check(b.Captured_call(captured, 1, 2, out))
	want(45)
	check(b.Captured_call(clone, 0, 0, out))
	want(42)
	check(b.Move_only_make(2, unsafe.Pointer(&moving)))
	check(b.Callable_live(out))
	want(1)
	check(b.Move_only_call(moving, 20, 20, out))
	want(42)
	check(b.Move_only_call(moving, 1, 2, out))
	want(45)
	check(b.Move_only_delete(unsafe.Pointer(&moving)))
	check(b.Callable_live(out))
	want(0)
	value = 999
	status, err = b.Captured_call(nil, 20, 22, out)
	if err != nil || status != cxx.InvalidArgument {
		panic("null callable receiver accepted")
	}
	want(0)
	fmt.Println(42)
}

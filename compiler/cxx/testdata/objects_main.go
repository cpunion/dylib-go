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

func expect(want, status int32, err error) {
	if err != nil || status != want {
		panic(fmt.Sprintf("C++ status %d: %v; want %d", status, err, want))
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
	factory, err := session.Resolve("object_new")
	if err != nil {
		panic(err)
	}
	lease, err := factory.Acquire()
	if err != nil {
		panic(err)
	}
	defer lease.Close()
	var first, second, clone, copy, bad, token, view, readonly unsafe.Pointer
	check(b.Object_new(20, unsafe.Pointer(&first)))
	check(b.Object_new(22, unsafe.Pointer(&second)))
	check(b.Object_new(13, unsafe.Pointer(&bad)))
	check(b.Readonly_new(22, unsafe.Pointer(&readonly)))
	defer func() {
		check(b.Token_delete(unsafe.Pointer(&token)))
		check(b.Readonly_delete(unsafe.Pointer(&readonly)))
		for _, p := range []*unsafe.Pointer{&first, &second, &clone, &copy, &bad} {
			check(b.Object_delete(unsafe.Pointer(p)))
		}
	}()
	var value int32
	out := unsafe.Pointer(&value)
	check(b.Object_sum(first, second, out))
	if value != 42 {
		panic("const reference argument")
	}
	check(b.Object_sum(first, readonly, out))
	if value != 42 {
		panic("constructed const-qualified alias")
	}
	check(b.Object_copied(first, second, out))
	if value != 42 {
		panic("class value argument")
	}
	check(b.Object_mixed(first, 0, second, nil, out))
	if value != 42 {
		panic("multiple object arguments at nonzero indices")
	}
	check(b.Object_sum(first, second, out))
	if value != 42 {
		panic("by-value mutation reached original")
	}
	check(b.Object_copy(second, unsafe.Pointer(&copy)))
	check(b.Object_delete(unsafe.Pointer(&copy)))
	check(b.Object_value_copy(second, unsafe.Pointer(&copy)))
	check(b.Object_clone(first, unsafe.Pointer(&clone)))
	if copy == nil || clone == nil || copy == second || clone == first {
		panic("missing independent native copies")
	}
	check(b.Object_mutate(clone, clone, 22, out))
	if value != 42 {
		panic("mutable reference argument")
	}
	check(b.Object_nullable(first, out))
	if value != 20 {
		panic("clone mutation reached original")
	}
	check(b.Object_self(first, unsafe.Pointer(&view)))
	if view != first {
		panic("overloaded operator& broke reference result")
	}
	check(b.Object_const_self(first, unsafe.Pointer(&view)))
	if view != first {
		panic("const reference result")
	}
	check(b.Object_pointer(second, unsafe.Pointer(&view)))
	if view != second {
		panic("typed pointer result")
	}
	check(b.Object_const_pointer(second, unsafe.Pointer(&view)))
	if view != second {
		panic("const typed pointer result")
	}
	check(b.Object_const_alias(second, unsafe.Pointer(&view)))
	if view != second {
		panic("const-qualified type alias result")
	}
	check(b.Object_pointer(nil, unsafe.Pointer(&view)))
	if view != nil {
		panic("nullable pointer result")
	}
	check(b.Object_nullable(nil, out))
	if value != 42 {
		panic("nullable const pointer argument")
	}
	status, err := b.Object_sum(first, nil, out)
	expect(cxx.InvalidArgument, status, err)
	if value != 0 {
		panic("null reference did not clear output")
	}
	status, err = b.Object_copied(first, bad, out)
	expect(cxx.Exception, status, err)
	if value != 0 {
		panic("throwing copy did not clear output")
	}
	occupied := clone
	status, err = b.Object_clone(first, unsafe.Pointer(&clone))
	expect(cxx.InvalidArgument, status, err)
	if clone != occupied {
		panic("owned result overwritten")
	}
	var failed unsafe.Pointer
	status, err = b.Object_clone(bad, unsafe.Pointer(&failed))
	expect(cxx.Exception, status, err)
	if failed != nil {
		panic("throwing result copy published an owner")
	}
	status, err = b.Object_copy(nil, unsafe.Pointer(&failed))
	expect(cxx.InvalidArgument, status, err)
	check(b.Token_make(42, unsafe.Pointer(&token)))
	check(b.Token_read(token, out))
	if value != 42 {
		panic("noncopyable C++17 value result")
	}
	check(b.Static_answer(out))
	if value != 42 {
		panic("static class with private destructor")
	}
	for _, p := range []*unsafe.Pointer{&first, &second, &clone, &copy, &bad} {
		check(b.Object_delete(unsafe.Pointer(p)))
	}
	check(b.Readonly_delete(unsafe.Pointer(&readonly)))
	check(b.Object_live(out))
	if value != 0 {
		panic("class argument/result owners leaked")
	}
	fmt.Println(42)
}

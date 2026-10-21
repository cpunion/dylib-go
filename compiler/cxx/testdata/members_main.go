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
	var owner unsafe.Pointer
	check(b.Member_new(20, unsafe.Pointer(&owner)))
	defer func() {
		check(b.Member_delete(unsafe.Pointer(&owner)))
		if owner != nil {
			panic("destructor did not consume native owner")
		}
	}()
	var result int32
	check(b.Member_add(owner, 22, unsafe.Pointer(&result)))
	if result != 42 {
		panic("discovered const overload")
	}
	check(b.Member_twice(21, unsafe.Pointer(&result)))
	if result != 42 {
		panic("discovered static member")
	}
	fmt.Println(result)
}

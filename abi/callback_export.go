//go:build libffi && cgo

package abi

/*
#include <stdint.h>
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"unsafe"
)

//export dylibgo_dispatch_callback
func dylibgo_dispatch_callback(handle C.uintptr_t, result, arguments unsafe.Pointer) {
	registered := enterCallbackThread()
	defer exitCallbackThread(registered)
	state := cgo.Handle(handle).Value().(*callbackState)
	// Every panic stays inside this exported C boundary, including failures
	// while decoding arguments or validating a handler's returned values.
	defer func() {
		if value := recover(); value != nil {
			state.record(fmt.Errorf("callback panic: %v", value))
		}
	}()
	state.backend.dispatch(state, result, arguments)
}

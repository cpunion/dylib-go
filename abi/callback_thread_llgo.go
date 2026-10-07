//go:build libffi && cgo && llgo

package abi

import (
	"sync"
	_ "unsafe"
)

// LLGo's runtime hooks attach a C-created thread to its collector and retain
// registration until that thread's native lifecycle ends. These hooks are
// present in the CI-pinned v1.0.6 runtime; keep them isolated from the Go backend.
//
//go:linkname llgoEnableCallbackThreads github.com/xgo-dev/llgo/runtime/internal/runtime.EnableForeignThreadRegistration
func llgoEnableCallbackThreads()

//go:linkname enterCallbackThread github.com/xgo-dev/llgo/runtime/internal/runtime.EnterForeignThread
func enterCallbackThread() bool

//go:linkname exitCallbackThread github.com/xgo-dev/llgo/runtime/internal/runtime.ExitForeignThread
func exitCallbackThread(bool)

var callbackThreadsOnce sync.Once

func initializeCallbackThreads() { callbackThreadsOnce.Do(llgoEnableCallbackThreads) }

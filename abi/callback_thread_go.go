//go:build libffi && cgo && !llgo

package abi

// The cgo-generated exported entry performs the gc runtime's foreign-thread
// transition. No additional collector registration is needed in ordinary Go.
func initializeCallbackThreads() {}
func enterCallbackThread() bool  { return false }
func exitCallbackThread(bool)    {}

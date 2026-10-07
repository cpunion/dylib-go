//go:build !cgo || (!darwin && !linux && !windows)

package native

import "fmt"

type Lifecycle struct{}

func NewLifecycle() (*Lifecycle, error) {
	return nil, fmt.Errorf("native lifecycle requires cgo or llgo")
}
func (*Lifecycle) Context() uintptr      { return 0 }
func (*Lifecycle) Helper(string) uintptr { return 0 }
func (*Lifecycle) Finalize()             {}
func (*Lifecycle) Close()                {}
func CallVoid(uintptr)                   { panic("native lifecycle requires cgo or llgo") }

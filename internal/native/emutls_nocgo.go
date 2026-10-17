//go:build !cgo || (!darwin && !linux && !windows)

package native

import "fmt"

type EmulatedTLS struct{}

func NewEmulatedTLS() (*EmulatedTLS, error) {
	return nil, fmt.Errorf("emulated TLS requires cgo or llgo")
}
func EmulatedTLSHelper() uintptr       { return 0 }
func (*EmulatedTLS) Add(uintptr) error { return fmt.Errorf("emulated TLS requires cgo or llgo") }
func (*EmulatedTLS) LiveValues() int   { return 0 }
func (*EmulatedTLS) Close()            {}

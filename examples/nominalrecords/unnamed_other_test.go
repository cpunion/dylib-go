//go:build cgo && (!darwin || !arm64)

package nominalrecords

import "testing"

func checkUnnamed(t *testing.T, b *Bindings) {}

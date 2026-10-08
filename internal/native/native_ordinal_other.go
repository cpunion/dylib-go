//go:build !windows

package native

func LookupOrdinal(uintptr, uint16) uintptr { return 0 }

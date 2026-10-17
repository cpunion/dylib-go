// Package coffname shares x86 COFF public-name normalization with declarations.
package coffname

import "strings"

// NormalizeI386 removes the extra C linker underscore. Vectorcall has no extra
// prefix, so its @@bytecount suffix preserves identifier-leading underscores.
func NormalizeI386(name string) string {
	if i := strings.LastIndex(name, "@@"); i >= 0 && i+2 < len(name) {
		digits := true
		for _, c := range name[i+2:] {
			digits = digits && c >= '0' && c <= '9'
		}
		if digits {
			return name
		}
	}
	return strings.TrimPrefix(name, "_")
}

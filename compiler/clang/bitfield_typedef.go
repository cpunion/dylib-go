package clang

import (
	"fmt"
	"strings"
)

func (r Record) hasBitfieldTypedefs() bool {
	for _, bit := range r.Bitfields {
		if bit.TypeAlignment != 0 {
			return true
		}
	}
	return false
}

// Typedef alignment can raise the record without moving a short POSIX
// bitfield. Reconstructed member attributes alone cannot preserve both facts.
func (t *cgoTypes) nativeAttributes(r Record) string {
	// MS unit attributes already preserve natural record alignment. Adding a
	// record-level request there would incorrectly bypass a parent's packing cap.
	if r.hasBitfieldTypedefs() && r.Layout.Alignment > r.Aligned {
		if t.msLayout && r.Packed {
			// GCC keeps the aligned unit offset but suppresses its contribution
			// to packed record alignment. MSVC needs the original natural
			// alignment so that enclosing pragma-packed records can cap it.
			return r.cAttributes() + fmt.Sprintf("\n#if defined(_WIN32) && !defined(_MSC_VER)\n__attribute__((aligned(%d)))\n#endif\n", r.Layout.Alignment)
		}
		if t.gnuPacking {
			r.Aligned = r.Layout.Alignment
		}
	}
	return r.cAttributes()
}

// Original-layout probes need typedef attributes on the base type, not on the
// member. MSVC also ignores some lowered requests and shared-unit requests.
func (r Record) bitfieldTypedefProbes(prefix string) (string, map[int]string) {
	var types strings.Builder
	kinds := make(map[int]string)
	for _, bit := range r.Bitfields {
		if bit.TypeAlignment == 0 {
			continue
		}
		kind := bitfieldBaseType(r.Description.Fields[bit.Position].Type.Type)
		name := fmt.Sprintf("%s_%d", prefix, bit.Position)
		fmt.Fprintf(&types, "typedef %s %s __attribute__((aligned(%d)));\n", kind, name, bit.TypeAlignment)
		kinds[bit.Position] = name
	}
	return types.String(), kinds
}

// A POSIX short field can remain inside the preceding allocation unit even
// when its typedef raises record alignment. A wider field starts a new unit.
// Preserve only boundaries that the compiler's byte mask actually observes.
func (h *Header) reconcileBitfieldTypedefs(r *Record) {
	if !h.Target.gnuPacking() {
		return
	}
	for i := range r.Bitfields {
		bit := &r.Bitfields[i]
		if bit.TypeAlignment == 0 {
			continue
		}
		alignment := bit.TypeAlignment
		if r.MaxFieldAlignment != 0 && alignment > r.MaxFieldAlignment {
			alignment = r.MaxFieldAlignment
		}
		if r.Packed || r.fieldPacked(bit.Position) {
			alignment = 1
		}
		if len(bit.Mask) == 0 || bit.Mask[0]&1 == 0 || r.Layout.Offsets[bit.Position]%alignment != 0 {
			alignment = 0
		}
		bit.StorageAlignment = alignment
	}
}

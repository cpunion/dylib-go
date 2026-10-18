package clang

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// Public records use Go fields. Only independently constructed C values cross
// the cgo boundary, so their padding/alignment need not match Go storage.
type cgoTypes struct {
	*directTypes
	cPrefix          string
	windows          bool
	msLayout         bool
	ms386            bool
	gnuPacking       bool
	gnuAMD64         bool
	darwinARM64      bool
	pointerTransport bool
	pointerSize      int
	transports       map[int]bool
}

func (t *cgoTypes) recordIndex(description abi.TypeDesc) (int, error) {
	name, err := t.directTypes.goType(description)
	if err != nil {
		return 0, err
	}
	for i := range t.records {
		if name == t.recordName(i) {
			return i, nil
		}
	}
	return 0, fmt.Errorf("clang: missing compiler record for cgo type")
}

func (t *cgoTypes) cRecordName(index int) string {
	return fmt.Sprintf("%s_record%d", t.cPrefix, index)
}

func (t *cgoTypes) cType(description abi.TypeDesc) (string, error) {
	if description.Type == abi.Struct {
		index, err := t.recordIndex(description)
		return t.cRecordName(index), err
	}
	if description.Type == abi.Pointer && description.Elem != nil && description.Elem.Type == abi.Struct {
		index, err := t.recordIndex(*description.Elem)
		return t.cRecordName(index) + " *", err
	}
	return cgoScalarType(description)
}

func (t *cgoTypes) goType(description abi.TypeDesc) (string, error) {
	if description.Type == abi.Struct {
		return t.directTypes.goType(description)
	}
	if description.Type == abi.Pointer && description.Elem != nil && description.Elem.Type == abi.Struct {
		return "unsafe.Pointer", nil
	}
	return directGoType(description)
}

func (t *cgoTypes) transport(description abi.TypeDesc) string {
	if description.Type == abi.Struct {
		index, _ := t.recordIndex(description)
		return t.cTransportName(index)
	}
	return cgoTransportType(description.Type)
}

func (t *cgoTypes) conversion(index int, toC bool) string {
	direction := "FromC"
	if toC {
		direction = "ToC"
	}
	return "_" + t.recordName(index) + direction
}

func (t *cgoTypes) cField(description abi.TypeDesc, name string) (string, error) {
	return t.cFieldType(description, name, false)
}

func (t *cgoTypes) cNativeField(record Record, index int) (string, error) {
	description := record.Description.Fields[index].Type
	name := fmt.Sprintf("f%d", index)
	if record.fieldTypeAlignment(index) == 0 || t.gnuAMD64 {
		return t.cField(description, name)
	}
	arrayDepth := 0
	for _, field := range record.FieldTypeAlignments {
		if field.Position == index {
			arrayDepth = field.ArrayDepth
		}
	}
	dimensions, _ := arrayDimensions(description)
	var suffix strings.Builder
	for _, length := range dimensions[:len(dimensions)-arrayDepth] {
		fmt.Fprintf(&suffix, "[%d]", length)
	}
	return t.cFieldAlias(record, index, arrayDepth) + " " + name + suffix.String(), nil
}

func (t *cgoTypes) cFieldAlias(record Record, index, arrayDepth int) string {
	// The record index disambiguates members with the same logical type.
	i, _ := t.recordIndex(record.Description)
	name := fmt.Sprintf("%s_field%d", t.cRecordName(i), index)
	if arrayDepth != 0 {
		name += fmt.Sprintf("_array%d", arrayDepth)
	}
	return name
}

func arrayDimensions(d abi.TypeDesc) ([]int, abi.TypeDesc) {
	var dimensions []int
	for d.Type == abi.Array {
		dimensions = append(dimensions, d.Len)
		d = *d.Elem
	}
	return dimensions, d
}

func (t *cgoTypes) writeFieldAliases(output *bytes.Buffer, record Record) error {
	if t.gnuAMD64 {
		return nil
	}
	position, depth := -1, 0
	var dimensions []int
	var kind string
	for _, field := range record.FieldTypeAlignments {
		if field.Position != position {
			var d abi.TypeDesc
			dimensions, d = arrayDimensions(record.Description.Fields[field.Position].Type)
			var err error
			kind, err = t.cType(d)
			if err != nil {
				return err
			}
			position, depth = field.Position, 0
		}
		alias := t.cFieldAlias(record, position, field.ArrayDepth)
		fmt.Fprintf(output, "typedef %s %s", kind, alias)
		for layer := field.ArrayDepth - 1; layer >= depth; layer-- {
			fmt.Fprintf(output, "[%d]", dimensions[len(dimensions)-1-layer])
		}
		fmt.Fprintf(output, " __attribute__((aligned(%d)));\n", field.Aligned)
		kind, depth = alias, field.ArrayDepth
	}
	return nil
}

func (t *cgoTypes) cFieldType(description abi.TypeDesc, name string, transport bool) (string, error) {
	var suffix strings.Builder
	for description.Type == abi.Array {
		fmt.Fprintf(&suffix, "[%d]", description.Len)
		description = *description.Elem
	}
	if description.Type == abi.Pointer && description.Elem != nil {
		return "", fmt.Errorf("typed record fields require opaque native addresses")
	}
	kind, err := t.cType(description)
	if err == nil && transport && description.Type == abi.Struct {
		kind = t.transport(description)
	}
	return kind + " " + name + suffix.String(), err
}

func (t *cgoTypes) writeCRecords(output *bytes.Buffer) error {
	// Header values can be assembled by callers, so emit dependencies first even
	// when their metadata order differs from Clang's parse order.
	seen := make(map[int]bool)
	var emit func(int) error
	emit = func(i int) error {
		if seen[i] {
			return nil
		}
		seen[i] = true
		record := t.records[i]
		if err := t.validateArrayTypedefStrides(record); err != nil {
			return err
		}
		for _, field := range record.Description.Fields {
			description := field.Type
			for description.Type == abi.Array {
				description = *description.Elem
			}
			if description.Type == abi.Struct {
				index, err := t.recordIndex(description)
				if err != nil {
					return err
				}
				if err := emit(index); err != nil {
					return err
				}
			}
		}
		var err error
		record, err = t.darwinPaddingRecord(record)
		if err != nil {
			return err
		}
		name := t.cRecordName(i)
		if err := t.writeFieldAliases(output, record); err != nil {
			return err
		}
		tailPadding := t.gnuAMD64 && gnuImplicitTailPadding(record) && t.sysVAlignedRecord(record, 0)
		if t.gnuAMD64 && len(record.Padding) != 0 && !tailPadding && record.Layout.Size <= 16 && t.sysVAlignedRecord(record, 0) {
			integers := t.sysVIntegerClasses(record, 0)
			for _, padding := range record.Padding {
				if padding.Width == 0 {
					continue
				}
				// Without an observation, require integer coverage of every lane.
				required := sysVClasses(0, record.Layout.Size)
				if len(padding.Mask) != 0 {
					required = sysVClasses(padding.Offset, uint64(len(padding.Mask)))
				}
				if required&^integers != 0 {
					return fmt.Errorf("clang: GNU AMD64 small unnamed bitfields in %s require an implicit-padding C bridge", record.Name)
				}
			}
		}
		if t.gnuAMD64 && record.Layout.Size <= 16 && len(record.Bitfields) == 0 && !tailPadding && t.sysVAlignedRecord(record, 0) {
			for _, padding := range record.Padding {
				if padding.Width != 0 && padding.Before == len(record.Description.Fields) {
					return fmt.Errorf("clang: GNU AMD64 small trailing unnamed bitfields in %s require an implicit-padding C bridge", record.Name)
				}
			}
		}
		prefixPadding := t.gnuPrefixPadding(record)
		plainBoundary := false
		if t.gnuPacking {
			for index, padding := range record.Padding {
				adapt := record.gnuPlainZeroBoundary(padding) && uint64(bitfieldTypeBits(padding.Type)/8) > record.Layout.Alignment
				plainBoundary = plainBoundary || adapt
				if adapt && padding.Gap != 0 && prefixPadding[index] == 0 {
					return fmt.Errorf("clang: GNU plain zero-width gaps in %s require an implicit-padding C bridge", record.Name)
				}
			}
		}
		if t.gnuAMD64 && (record.MaxFieldAlignment != 0 || record.hasBitfieldTypedefs()) && record.Layout.Size <= 16 && t.sysVAlignedRecord(record, 0) {
			for index, padding := range record.Padding {
				if padding.Width != 0 && padding.TypeAlignment != 0 && padding.Gap != 0 && hasFloatingFields(record.Description) {
					return fmt.Errorf("clang: GNU AMD64 small unnamed typedef gaps in %s require an implicit-padding C bridge", record.Name)
				}
				if padding.Width == 0 && (padding.TypeAlignment != 0 || padding.Aligned > record.MaxFieldAlignment) && padding.requestedAlignment() > record.Layout.Alignment && padding.Gap != 0 && (!tailPadding || padding.Before != len(record.Description.Fields)) && prefixPadding[index] == 0 {
					return fmt.Errorf("clang: GNU AMD64 small zero-width gaps in %s require an implicit-padding C bridge", record.Name)
				}
			}
		}
		bodyName := name
		if tailPadding {
			bodyName += "_body"
		}
		members := recordMembers(record)
		var packedUnits []bitfieldUnit
		if t.msLayout {
			packedUnits = packedBitfieldUnits(record, members)
		}
		packedBridge := t.msLayout && record.Packed && (len(record.Padding) != 0 || record.hasBitfields() && len(record.FieldAlignments) != 0)
		pragmaBridge := t.msLayout && record.MaxFieldAlignment != 0
		if packedBridge || pragmaBridge {
			fmt.Fprint(output, "#if defined(_WIN32) && !defined(_MSC_VER)\n")
			if pragmaBridge {
				var err error
				if record.hasBitfields() {
					err = t.writePragmaBitfields(output, i)
				} else {
					err = t.writePragmaRecord(output, i)
				}
				if err != nil {
					return err
				}
			} else {
				if err := t.writePackedPadding(output, i); err != nil {
					return err
				}
			}
			fmt.Fprint(output, "#else\n")
		}
		bridgePacking := record.MaxFieldAlignment
		// GCC can retain an unnamed unit's alignment under a pragma even
		// when the producer's packed record has a lower root alignment.
		if t.gnuPacking && (record.hasPaddingTypedefs() || record.Packed && len(record.Padding) != 0) && bridgePacking > record.Layout.Alignment {
			bridgePacking = record.Layout.Alignment
		}
		if (plainBoundary || len(prefixPadding) != 0) && (bridgePacking == 0 || bridgePacking > record.Layout.Alignment) {
			bridgePacking = record.Layout.Alignment
		}
		if bridgePacking != 0 {
			fmt.Fprintf(output, "#pragma pack(push,%d)\n", bridgePacking)
		}
		// MinGW ignores a record's packed attribute for MS-layout bitfields.
		// Preserve the packed allocation units with a scoped pragma. Explicit
		// aligned attributes restore record alignment; size/offset/mask checks
		// remain authoritative.
		packedBits := t.msLayout && record.Packed && record.hasBitfields() && (record.Layout.Alignment == 1 || record.Aligned != 0 || len(record.FieldAlignments) != 0)
		if packedBits {
			fmt.Fprint(output, "#if defined(_WIN32)\n#pragma pack(push,1)\n#endif\n")
		}
		fmt.Fprint(output, "typedef struct ")
		declarationRecord := record
		if plainBoundary && declarationRecord.Aligned < record.Layout.Alignment {
			// A packed producer can derive its root alignment from the zero
			// width. Its capped anonymous replacement must restore that root.
			declarationRecord.Aligned = record.Layout.Alignment
		}
		if t.gnuPacking && record.MaxFieldAlignment != 0 {
			for _, padding := range record.Padding {
				if padding.Width == 0 && padding.Aligned > record.MaxFieldAlignment && padding.Aligned > record.Layout.Alignment {
					// Zero widths contribute to record alignment differently
					// across GNU targets. Preserve the producer's observation.
					declarationRecord.Aligned = record.Layout.Alignment
					break
				}
			}
		}
		if tailPadding {
			declarationRecord.Aligned = record.Layout.Size
		}
		fmt.Fprint(output, t.nativeAttributes(declarationRecord))
		fmt.Fprint(output, "{\n")
		// Open outer boundaries first. Closing each prefix at its measured
		// zero-width declaration leaves its gap as implicit tail padding.
		for index := len(record.Padding) - 1; index >= 0; index-- {
			if alignment := prefixPadding[index]; alignment != 0 {
				prefix := Record{Packed: record.Packed, Aligned: alignment}
				fmt.Fprintf(output, "struct %s{\n", prefix.cAttributes())
			}
		}
		var packedBoundaries []int
		unitStarts := bitfieldUnitStarts(record, members)
		for position, member := range members {
			for _, unit := range packedUnits {
				if unit.first == position {
					alignment := uint64(0)
					if member.field >= 0 {
						alignment = record.fieldAlignment(member.field)
						if bit := record.bitfieldAt(member.field); bit != nil && bit.StorageAlignment > alignment {
							alignment = bit.StorageAlignment
						}
					} else {
						alignment = record.Padding[member.padding].Aligned
						if record.Padding[member.padding].StorageAlignment > alignment {
							alignment = record.Padding[member.padding].StorageAlignment
						}
					}
					beginPackedBitfieldUnit(output, alignment)
				}
			}
			if member.padding >= 0 {
				p := t.nativePadding(record.Padding[member.padding])
				if tailPadding && p.Width != 0 && p.Before == len(record.Description.Fields) {
					continue
				}
				if t.gnuPacking && record.gnuPlainZeroBoundary(p) && uint64(bitfieldTypeBits(p.Type)/8) > record.Layout.Alignment {
					// Keep the zero width inside a capped anonymous member. It
					// affects i386 float expansion and SysV integer coercion even
					// when the byte gap is empty, but must not raise GCC's root.
					if prefixPadding[member.padding] != 0 {
						fmt.Fprintf(output, "%s :0;\n};\n", bitfieldBaseType(p.Type))
						// Clang also needs the original outer declaration to
						// retain trailing integer coercion widths. It has no
						// extra byte gap here; GCC must keep it inside the cap.
						fmt.Fprintf(output, "#if defined(__clang__)\n%s :0;\n#endif\n", bitfieldBaseType(p.Type))
					} else {
						fmt.Fprintf(output, "struct {%s :0;};\n", bitfieldBaseType(p.Type))
					}
					continue
				}
				if prefixPadding[member.padding] != 0 {
					fmt.Fprint(output, "};\n")
				}
				if t.gnuPacking && p.TypeAlignment != 0 && (p.Width == 0 || p.Gap != 0) {
					gap := p.Gap
					if prefixPadding[member.padding] != 0 || tailPadding && p.Before == len(record.Description.Fields) {
						gap = 0
					}
					writeGNUPragmaBoundary(output, gap)
					if p.Width == 0 {
						// Retain the base zero-width declaration when its natural
						// boundary already fits; it can affect SysV coercion width.
						natural := uint64(bitfieldTypeBits(p.Type) / 8)
						if t.gnuAMD64 && natural != 0 && p.Offset%natural == 0 {
							fmt.Fprintf(output, "%s :0;\n", bitfieldBaseType(p.Type))
						}
						continue
					}
				}
				if t.gnuPacking && record.MaxFieldAlignment != 0 && p.Aligned > record.MaxFieldAlignment && (p.Width != 0 || p.Aligned > record.Layout.Alignment) {
					if p.Width == 0 {
						gap := p.Gap
						if prefixPadding[member.padding] != 0 || tailPadding && p.Before == len(record.Description.Fields) {
							gap = 0
						}
						writeGNUPragmaBoundary(output, gap)
						continue
					}
					// Clang ignores an over-cap nonzero request entirely.
					p.Aligned = 0
				}
				boundary := false
				// MinGW zero-width fields can bypass a pragma's alignment cap.
				// Replace only active boundaries; leading/consecutive requests
				// must retain MSVC's ignored-field behavior.
				if t.msLayout && !record.Packed && (p.Packed || p.Aligned != 0 || record.MaxFieldAlignment != 0) && p.Width == 0 && position > 0 {
					_, previousWidth := members[position-1].bitfield(record)
					boundary = previousWidth != 0
				}
				for _, unit := range packedUnits {
					boundary = boundary || p.Width == 0 && unit.last+1 == position
				}
				if boundary {
					packedBoundaries = append(packedBoundaries, member.padding)
					writePackedUnitBoundary(output, p, member.padding)
				} else {
					kind, err := cgoScalarType(abi.TypeDesc{Type: p.Type})
					if err != nil {
						return err
					}
					if t.msLayout && p.Aligned != 0 && !unitStarts[position] {
						// MSVC ignores alignment within an existing unit, and on
						// leading/consecutive zero-width fields. MinGW can still
						// raise record alignment, so suppress only that request.
						plain := p
						plain.Aligned = 0
						fmt.Fprintf(output, "#if defined(_WIN32) && !defined(_MSC_VER)\n%s :%d%s;\n#else\n%s :%d%s;\n#endif\n", kind, p.Width, plain.cAttributes(), kind, p.Width, p.cAttributes())
					} else {
						fmt.Fprintf(output, "%s :%d%s;\n", kind, p.Width, p.cAttributes())
					}
				}
			} else {
				j := member.field
				field := record.Description.Fields[j]
				declaration, err := t.cNativeField(record, j)
				if err != nil {
					return fmt.Errorf("clang: %s.%s: %w", record.Name, field.Name, err)
				}
				if bit := record.bitfieldAt(j); bit != nil {
					declaration += fmt.Sprintf(":%d", bit.Width)
				}
				alignment := record.fieldAlignment(j)
				if t.gnuPacking && record.bitfieldAt(j) != nil && record.MaxFieldAlignment != 0 && alignment > record.MaxFieldAlignment {
					alignment = 0
				}
				t.writeNativeField(output, record, j, declaration, fieldCAttributes(record.fieldPacked(j), alignment))
			}
			for _, unit := range packedUnits {
				if unit.last == position {
					endPackedBitfieldUnit(output)
				}
			}
		}
		if record.ArgumentAlignment != 0 {
			// A zero-length naturally aligned integer member preserves AAPCS64
			// even-register allocation without introducing a value or moving fields.
			fmt.Fprint(output, "__int128 argument_alignment[0];\n")
		}
		fmt.Fprintf(output, "} %s;\n", bodyName)
		if tailPadding {
			fmt.Fprintf(output, "typedef %s %s __attribute__((aligned(%d)));\n", bodyName, name, record.Layout.Alignment)
		}
		if bridgePacking != 0 {
			fmt.Fprint(output, "#pragma pack(pop)\n")
		}
		if packedBits {
			fmt.Fprint(output, "#if defined(_WIN32)\n#pragma pack(pop)\n#endif\n")
		}
		if packedBridge || pragmaBridge {
			fmt.Fprint(output, "#endif\n")
		}
		if len(packedBoundaries) != 0 {
			fmt.Fprint(output, "#if defined(_WIN32) && !defined(_MSC_VER)\n")
			for _, index := range packedBoundaries {
				fmt.Fprintf(output, "_Static_assert(offsetof(%s,boundary%d)==%d,\"incompatible zero-width boundary\");\n", name, index, record.Padding[index].Offset)
			}
			fmt.Fprint(output, "#endif\n")
		}
		if len(record.Bitfields) != 0 {
			t.writeBitfieldMasks(output, i)
		}
		fmt.Fprintf(output, "static uint64_t %s_layout(int32_t index) {\nswitch(index) {\ncase 0:return sizeof(%s);\ncase 1:return _Alignof(%s);\n", name, name, name)
		for j := range record.Description.Fields {
			if record.bitfieldAt(j) != nil {
				fmt.Fprintf(output, "case %d:return %s_mask(%d,UINT64_MAX);\n", j+2, name, j)
			} else {
				fmt.Fprintf(output, "case %d:return offsetof(%s,f%d);\n", j+2, name, j)
			}
		}
		fmt.Fprint(output, "default:return UINT64_MAX;\n}\n}\n")
		if t.needsTransport(i) {
			if err := t.writeTransport(output, i); err != nil {
				return err
			}
		}
		return nil
	}
	for i := range t.records {
		if err := emit(i); err != nil {
			return err
		}
	}
	return nil
}

func (t *cgoTypes) writeCLayoutChecks(output *bytes.Buffer) {
	for i, record := range t.records {
		name, layout := t.cRecordName(i), record.Layout
		fmt.Fprintf(output, "if uint64(C.%s_layout(0)) != %d || uint64(C.%s_layout(1)) != %d", name, layout.Size, name, layout.Alignment)
		for j, offset := range layout.Offsets {
			fmt.Fprintf(output, " || uint64(C.%s_layout(%d)) != %d", name, j+2, offset)
		}
		for _, field := range record.Bitfields {
			for j, mask := range field.Mask {
				fmt.Fprintf(output, " || uint64(C.%s_mask(%d,%d)) != %d", name, field.Position, layout.Offsets[field.Position]+uint64(j), mask)
			}
		}
		fmt.Fprintf(output, " { return nil,fmt.Errorf(%q) }\n", "clang: compiler/cgo layout mismatch for "+record.Name)
	}
}

func (t *cgoTypes) writeConversions(output *bytes.Buffer) error {
	for i, record := range t.records {
		for _, toC := range []bool{true, false} {
			from, to := t.recordName(i), "C."+t.cTransportName(i)
			if !toC {
				from, to = to, from
			}
			fmt.Fprintf(output, "\nfunc %s(value %s) (result %s) {\n", t.conversion(i, toC), from, to)
			for j, field := range record.Description.Fields {
				goField, cField := "."+t.fields[i][j], fmt.Sprintf(".f%d", j)
				destination, source := "result"+cField, "value"+goField
				if !toC {
					destination, source = "result"+goField, "value"+cField
				}
				if err := t.writeFieldCopy(output, field.Type, destination, source, toC, 0); err != nil {
					return err
				}
			}
			fmt.Fprint(output, "return result\n}\n")
		}
	}
	return nil
}

func (t *cgoTypes) writeFieldCopy(output *bytes.Buffer, description abi.TypeDesc, destination, source string, toC bool, depth int) error {
	if description.Type == abi.Array {
		index := fmt.Sprintf("i%d", depth)
		fmt.Fprintf(output, "for %s := range %d {\n", index, description.Len)
		if err := t.writeFieldCopy(output, *description.Elem, destination+"["+index+"]", source+"["+index+"]", toC, depth+1); err != nil {
			return err
		}
		fmt.Fprint(output, "}\n")
		return nil
	}
	var expression string
	if description.Type == abi.Struct {
		index, err := t.recordIndex(description)
		if err != nil {
			return err
		}
		expression = t.conversion(index, toC) + "(" + source + ")"
	} else if description.Type == abi.Pointer {
		if _, err := t.cType(description); err != nil {
			return err
		}
		expression = source
	} else {
		kind, err := t.goType(description)
		if toC {
			kind, err = t.cType(description)
			kind = "C." + kind
		}
		if err != nil {
			return err
		}
		// cgo names the native float/double types C.float and C.double.
		expression = kind + "(" + source + ")"
	}
	fmt.Fprintf(output, "%s = %s\n", destination, expression)
	return nil
}

func containsNativePointer(description abi.TypeDesc) bool {
	if description.Type == abi.Pointer {
		return true
	}
	if description.Elem != nil && containsNativePointer(*description.Elem) {
		return true
	}
	for _, field := range description.Fields {
		if containsNativePointer(field.Type) {
			return true
		}
	}
	return false
}

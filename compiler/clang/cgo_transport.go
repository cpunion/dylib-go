package clang

import (
	"bytes"
	"fmt"

	"github.com/cpunion/dylib-go/abi"
)

// Keep attributed records entirely inside compiled C. Both cgo and llgo receive an
// ordinary C transport record, whose fields are copied rather than reinterpreted.
// A parent containing packed records needs a transport even when it is unpacked.
func (t *cgoTypes) needsTransport(index int) bool {
	if value, ok := t.transports[index]; ok {
		return value
	}
	if t.transports == nil {
		t.transports = make(map[int]bool)
	}
	record := t.records[index]
	needed := record.requiresCAdapter() || t.msFloatReturn(record)
	for _, field := range record.Description.Fields {
		d := field.Type
		for d.Type == abi.Array {
			d = *d.Elem
		}
		if d.Type == abi.Struct {
			child, _ := t.recordIndex(d)
			needed = t.needsTransport(child) || needed
		}
	}
	t.transports[index] = needed
	return needed
}

func (t *cgoTypes) cConversion(index int, toNative bool) string {
	direction := "_to_transport"
	if toNative {
		direction = "_from_transport"
	}
	return t.cRecordName(index) + direction
}

func (t *cgoTypes) cTransportName(index int) string {
	name := t.cRecordName(index)
	if t.needsTransport(index) {
		name += "_transport"
	}
	return name
}

func (t *cgoTypes) writeTransport(output *bytes.Buffer, index int) error {
	record := t.records[index]
	native, transport := t.cRecordName(index), t.cTransportName(index)
	fmt.Fprint(output, "typedef struct {\n")
	for j, field := range record.Description.Fields {
		declaration, err := t.cFieldType(field.Type, fmt.Sprintf("f%d", j), true)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "%s;\n", declaration)
	}
	fmt.Fprintf(output, "} %s;\n", transport)
	for _, toNative := range []bool{true, false} {
		from, to := transport, native
		if !toNative {
			from, to = native, transport
		}
		initializer := "{0}"
		if toNative && t.emptyNativeInitializer(index) {
			initializer = "{}"
		}
		fmt.Fprintf(output, "static %s %s(%s value) {\n%s result = %s;\n", to, t.cConversion(index, toNative), from, to, initializer)
		for j, field := range record.Description.Fields {
			member := fmt.Sprintf(".f%d", j)
			t.writeTransportCopy(output, field.Type, "result"+member, "value"+member, toNative, 0)
		}
		fmt.Fprint(output, "return result;\n}\n")
	}
	return nil
}

// A leading wrapped unit may have only unnamed fields. Brace elision descends
// into the first record/array member, so {0} is also invalid in its parents.
// GCC/Clang's empty initializer clears the aggregate without targeting that unit.
func (t *cgoTypes) emptyNativeInitializer(index int) bool {
	r := t.records[index]
	if t.gnuPacking {
		for _, p := range r.Padding {
			if p.Before == 0 && r.gnuPlainZeroBoundary(p) && uint64(bitfieldTypeBits(p.Type)/8) > r.Layout.Alignment {
				return true
			}
		}
	}
	if t.msLayout && len(r.Padding) != 0 && r.Padding[0].Before == 0 && (r.MaxFieldAlignment != 0 || !r.Packed && r.Padding[0].Packed) {
		return true
	}
	first := r.Description.Fields[0].Type
	for first.Type == abi.Array {
		first = *first.Elem
	}
	if first.Type == abi.Struct {
		child, _ := t.recordIndex(first)
		return t.emptyNativeInitializer(child)
	}
	return false
}

func (t *cgoTypes) writeTransportCopy(output *bytes.Buffer, d abi.TypeDesc, destination, source string, toNative bool, depth int) {
	if d.Type == abi.Array {
		index := fmt.Sprintf("i%d", depth)
		fmt.Fprintf(output, "for (uint32_t %s = 0; %s < %d; ++%s) {\n", index, index, d.Len, index)
		t.writeTransportCopy(output, *d.Elem, destination+"["+index+"]", source+"["+index+"]", toNative, depth+1)
		fmt.Fprint(output, "}\n")
		return
	}
	if d.Type == abi.Struct {
		index, _ := t.recordIndex(d)
		if t.needsTransport(index) {
			source = t.cConversion(index, toNative) + "(" + source + ")"
		}
	}
	fmt.Fprintf(output, "%s = %s;\n", destination, source)
}

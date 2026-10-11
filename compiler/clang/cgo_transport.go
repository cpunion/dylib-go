package clang

import (
	"bytes"
	"fmt"

	"github.com/cpunion/dylib-go/abi"
)

// Keep packed records entirely inside compiled C. Both cgo and llgo receive an
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
	needed := record.Packed
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
		fmt.Fprintf(output, "static %s %s(%s value) {\n%s result = {0};\n", to, t.cConversion(index, toNative), from, to)
		for j, field := range record.Description.Fields {
			member := fmt.Sprintf(".f%d", j)
			t.writeTransportCopy(output, field.Type, "result"+member, "value"+member, toNative, 0)
		}
		fmt.Fprint(output, "return result;\n}\n")
	}
	return nil
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

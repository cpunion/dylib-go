package dylib

import (
	"encoding/binary"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// Validate before encoding: ignoring scalar-only empty metadata must never let
// malformed descriptors collide with a previously prepared valid signature.
// Length prefixes make arbitrary field names unambiguous. The key is private
// to this cache; it is not a persistent ABI or a cross-session identifier.
func bindingKey(s abi.Signature) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	b.Grow(8 + len(s.Args))
	b.WriteByte(byte(s.Convention))
	if s.Variadic {
		b.WriteByte(1)
	} else {
		b.WriteByte(0)
	}
	bindingNumber(&b, uint64(s.FixedArgs))
	bindingType(&b, s.ReturnType())
	bindingNumber(&b, uint64(len(s.Args)))
	for i := range s.Args {
		bindingType(&b, s.ArgumentType(i))
	}
	return b.String(), nil
}

func bindingNumber(b *strings.Builder, n uint64) {
	var buffer [binary.MaxVarintLen64]byte
	count := binary.PutUvarint(buffer[:], n)
	b.Write(buffer[:count])
}

func bindingType(b *strings.Builder, d abi.TypeDesc) {
	b.WriteByte(byte(d.Type))
	switch d.Type {
	case abi.Struct:
		bindingNumber(b, uint64(len(d.Fields)))
		for _, field := range d.Fields {
			bindingNumber(b, uint64(len(field.Name)))
			b.WriteString(field.Name)
			bindingType(b, field.Type)
		}
	case abi.Pointer:
		if d.Elem == nil {
			b.WriteByte(0)
		} else {
			b.WriteByte(1)
			bindingType(b, *d.Elem)
		}
	case abi.Array:
		bindingNumber(b, uint64(d.Len))
		bindingType(b, *d.Elem)
	}
}

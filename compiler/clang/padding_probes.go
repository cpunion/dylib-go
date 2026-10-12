package clang

import (
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// Observe the byte boundary immediately after each zero-width field. Prefix
// records preserve producer field types via typeof and use the same packing
// and compiler flags. No name or offsetof is applied to a bitfield itself.
func (h *Header) paddingProbes() (declarations, constants string) {
	var types, values strings.Builder
	for i, r := range h.Records {
		var body strings.Builder
		padding := 0
		for j := 0; j <= len(r.Description.Fields); j++ {
			for padding < len(r.Padding) && r.Padding[padding].Before == j {
				p := r.Padding[padding]
				fmt.Fprintf(&body, "%s :%d;\n", bitfieldBaseType(p.Type), p.Width)
				if p.Width == 0 {
					name := fmt.Sprintf("dylib_padding_%d_%d", i, padding)
					fmt.Fprint(&types, "typedef struct ")
					if r.Packed {
						fmt.Fprint(&types, "__attribute__((packed)) ")
					}
					fmt.Fprintf(&types, "{\n%s unsigned char marker;\n} %s;\n", body.String(), name)
					fmt.Fprintf(&values, "dylib_record_%d_padding_%d=__builtin_offsetof(%s,marker),\n", i, padding, name)
				}
				padding++
			}
			if j == len(r.Description.Fields) {
				break
			}
			field := r.Description.Fields[j]
			if bit := r.bitfieldAt(j); bit != nil {
				fmt.Fprintf(&body, "%s f%d:%d;\n", bitfieldBaseType(field.Type.Type), j, bit.Width)
			} else {
				fmt.Fprintf(&body, "__typeof__(((%s *)0)->%s) f%d;\n", r.Name, field.Name, j)
			}
		}
	}
	return types.String(), values.String()
}

func bitfieldBaseType(kind abi.Type) string {
	switch kind {
	case abi.Bool:
		return "_Bool"
	case abi.I8:
		return "signed char"
	case abi.U8:
		return "unsigned char"
	case abi.I16:
		return "short"
	case abi.U16:
		return "unsigned short"
	case abi.I32:
		return "int"
	case abi.U32:
		return "unsigned int"
	case abi.I64:
		return "long long"
	case abi.U64:
		return "unsigned long long"
	}
	return "void" // Parse validates integral/Boolean bases before these probes.
}

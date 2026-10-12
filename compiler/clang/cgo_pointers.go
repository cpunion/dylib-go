package clang

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// Emit children first so nested argument/result typedefs retain their complete
// C prototypes and independent calling conventions. Go transports addresses.
func (h *Header) writeCgoFunctionPointer(output *bytes.Buffer, pointer FunctionPointer, name string, mapper *cgoTypes) (string, error) {
	s := pointer.Signature
	attribute, err := h.cgoAttribute(s)
	if err != nil {
		return "", err
	}
	boundary := make(map[int]string)
	for _, child := range pointer.FunctionPointers {
		kind, err := h.writeCgoFunctionPointer(output, child, fmt.Sprintf("%s_pointer%d", name, child.Position+1), mapper)
		if err != nil {
			return "", err
		}
		boundary[child.Position] = kind
	}
	kindAt := func(position int, description abi.TypeDesc) (string, error) {
		if kind, ok := boundary[position]; ok {
			return kind, nil
		}
		if position >= 0 {
			return mapper.cParameterType(description)
		}
		return mapper.cType(description)
	}
	result, err := kindAt(-1, s.ReturnType())
	if err != nil {
		return "", err
	}
	count := len(s.Args)
	if s.Variadic {
		count = s.FixedArgs
	}
	var parameters []string
	for i := 0; i < count; i++ {
		kind, err := kindAt(i, s.ArgumentType(i))
		if err != nil {
			return "", err
		}
		parameters = append(parameters, kind)
	}
	if s.Variadic {
		parameters = append(parameters, "...")
	} else if count == 0 {
		parameters = append(parameters, "void")
	}
	fmt.Fprintf(output, "typedef %s (%s*%s)(%s);\n", result, attribute, name, strings.Join(parameters, ","))
	return name + strings.Repeat("*", pointer.pointerDepth()-1), nil
}

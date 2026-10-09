package clang

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// CgoSource emits typed C cdecl bindings using fixed cgo bridges, without libffi.
// Go and llgo can compile the generated source. Scalars, opaque native pointers
// and function-pointer addresses are supported. WithTail specifies a concrete
// variadic call shape; the C compiler performs default argument promotions.
// Records and non-cdecl conventions require another adapter. The caller owns the
// Session, native storage and any returned or passed function-pointer lifetimes.
func (h *Header) CgoSource(packageName, bindingType string) ([]byte, error) {
	if err := validateBindingName(h, bindingType); err != nil {
		return nil, err
	}
	metadata := "_" + bindingType + "Declarations"
	source, err := h.GoSource(packageName, metadata)
	if err != nil {
		return nil, err
	}
	// Encoding the Go identifier also supports Unicode without C name collisions.
	prefix := fmt.Sprintf("dylib_go_%x", []byte(bindingType))
	var preamble, methods bytes.Buffer
	fmt.Fprint(&preamble, "/*\n#include <stdint.h>\n")
	seen := make(map[string]bool)
	needsUnsafe := false
	for i, function := range h.Functions {
		needsUnsafe = needsUnsafe || function.Signature.Result == abi.Pointer
		for _, argument := range function.Signature.Args {
			needsUnsafe = needsUnsafe || argument == abi.Pointer
		}
		name := directName(function.Name)
		if seen[name] {
			return nil, fmt.Errorf("clang: generated method name %s is ambiguous", name)
		}
		seen[name] = true
		helper := fmt.Sprintf("%s_call%d", prefix, i)
		if err := h.writeCgoBridge(&preamble, &methods, function, bindingType, name, helper, i); err != nil {
			return nil, fmt.Errorf("clang: %s: %w", function.Name, err)
		}
	}
	fmt.Fprint(&preamble, "*/\nimport \"C\"\n\n")
	// A separate C import keeps the preamble adjacent to cgo's special import.
	fmt.Fprint(&preamble, "import (\ndylib \"github.com/cpunion/dylib-go\"\n")
	if needsUnsafe {
		fmt.Fprint(&preamble, "\"unsafe\"\n")
	}
	source = bytes.Replace(source, []byte("\nimport ("), preamble.Bytes(), 1)
	var output bytes.Buffer
	output.Write(source)
	fmt.Fprintf(&output, "\n// %s retains symbol handles; its caller owns the Session.\ntype %s struct {\n", bindingType, bindingType)
	for i := range h.Functions {
		fmt.Fprintf(&output, "symbol%d *dylib.Symbol\n", i)
	}
	fmt.Fprintf(&output, "}\n\n// New%s validates the host and resolves each requested export.\nfunc New%s(session *dylib.Session) (*%s,error) {\nif session == nil { return nil,dylib.ErrClosed }\nif err := %s.Target.CheckHost(); err != nil { return nil,err }\nb := &%s{}\n", bindingType, bindingType, bindingType, metadata, bindingType)
	for i, function := range h.Functions {
		fmt.Fprintf(&output, "symbol%d,err := session.Resolve(%q)\nif err != nil { return nil,err }\nb.symbol%d = symbol%d\n", i, function.Symbol, i, i)
	}
	fmt.Fprint(&output, "return b,nil\n}\n")
	output.Write(methods.Bytes())
	source = bytes.Replace(output.Bytes(), []byte("\npackage "), []byte("\n//go:build cgo\n\npackage "), 1)
	return format.Source(source)
}

func cgoScalarType(d abi.TypeDesc) (string, error) {
	if d.Elem != nil || d.Type == abi.Struct || d.Type == abi.Array {
		return "", fmt.Errorf("cgo generation supports scalars and opaque native pointers")
	}
	return map[abi.Type]string{abi.Void: "void", abi.I8: "int8_t", abi.U8: "uint8_t", abi.I16: "int16_t", abi.U16: "uint16_t", abi.I32: "int32_t", abi.U32: "uint32_t", abi.I64: "int64_t", abi.U64: "uint64_t", abi.F32: "float", abi.F64: "double", abi.Bool: "_Bool", abi.Pointer: "void *"}[d.Type], nil
}

// Full-width integer transport avoids depending on a host compiler's narrow
// parameter extension rules. C restores the declared width before native calls.
func cgoTransportType(kind abi.Type) string {
	switch kind {
	case abi.I8, abi.I16, abi.Bool:
		return "int32_t"
	case abi.U8, abi.U16:
		return "uint32_t"
	}
	name, _ := cgoScalarType(abi.TypeDesc{Type: kind})
	return name
}

func cgoPrototype(s abi.Signature) (string, string, error) {
	if s.Convention != abi.Default && s.Convention != abi.CDecl {
		return "", "", fmt.Errorf("cgo bridges require cdecl prototypes")
	}
	result, err := cgoScalarType(s.ReturnType())
	if err != nil {
		return "", "", err
	}
	count := len(s.Args)
	if s.Variadic {
		count = s.FixedArgs
	}
	var parameters []string
	for i := 0; i < count; i++ {
		kind, err := cgoScalarType(s.ArgumentType(i))
		if err != nil {
			return "", "", err
		}
		parameters = append(parameters, kind)
	}
	if s.Variadic {
		parameters = append(parameters, "...")
	} else if count == 0 {
		parameters = append(parameters, "void")
	}
	return result, strings.Join(parameters, ","), nil
}

func (h *Header) writeCgoBridge(c, goCode *bytes.Buffer, function Declaration, bindingType, method, helper string, index int) error {
	s := function.Signature
	resultC, _, err := cgoPrototype(s)
	if err != nil {
		return err
	}
	resultGo, _ := directGoType(s.ReturnType())
	// Force cdecl on Windows i386 even if the surrounding C flags change defaults.
	attribute := ""
	if h.Target.OS == "windows" && h.Target.Arch == "386" {
		attribute = "__attribute__((cdecl)) "
	}
	boundaryC := make(map[int]string)
	for _, pointer := range function.FunctionPointers {
		result, parameters, err := cgoPrototype(pointer.Signature)
		if err != nil {
			return fmt.Errorf("function pointer %d: %w", pointer.Position, err)
		}
		name := fmt.Sprintf("%s_pointer%d", helper, pointer.Position+1)
		fmt.Fprintf(c, "typedef %s (%s*%s)(%s);\n", result, attribute, name, parameters)
		boundaryC[pointer.Position] = name
	}
	var prototype, bridge, callArgs, goParams, goArgs, conversions []string
	bridge = append(bridge, "uintptr_t address")
	goArgs = append(goArgs, "C.uintptr_t(address)")
	count := len(s.Args)
	if s.Variadic {
		count = s.FixedArgs
	}
	for i := range s.Args {
		kind, err := cgoScalarType(s.ArgumentType(i))
		if err != nil {
			return fmt.Errorf("argument %d: %w", i, err)
		}
		goKind, _ := directGoType(s.ArgumentType(i))
		goParams = append(goParams, fmt.Sprintf("p%d %s", i, goKind))
		transport := cgoTransportType(s.Args[i])
		bridge = append(bridge, fmt.Sprintf("%s p%d", transport, i))
		argument := fmt.Sprintf("p%d", i)
		if s.Args[i] == abi.Bool {
			conversions = append(conversions, fmt.Sprintf("var c%d C.int32_t\nif p%d { c%d = 1 }\n", i, i, i))
			goArgs = append(goArgs, fmt.Sprintf("c%d", i))
		} else if kind != "void *" {
			goArgs = append(goArgs, fmt.Sprintf("C.%s(p%d)", transport, i))
		} else {
			goArgs = append(goArgs, argument)
		}
		if transport != kind {
			argument = "(" + kind + ")" + argument
		}
		if callback, ok := boundaryC[i]; ok {
			kind = callback
			argument = "(" + callback + ")" + argument
		}
		callArgs = append(callArgs, argument)
		if i < count {
			prototype = append(prototype, kind)
		}
	}
	if s.Variadic {
		prototype = append(prototype, "...")
	} else if count == 0 {
		prototype = append(prototype, "void")
	}
	prototypeResult := resultC
	if callback, ok := boundaryC[-1]; ok {
		prototypeResult = callback
	}
	fmt.Fprintf(c, "typedef %s (%s*%s_function)(%s);\nstatic %s %s(%s) {\n", prototypeResult, attribute, helper, strings.Join(prototype, ","), cgoTransportType(s.Result), helper, strings.Join(bridge, ","))
	if s.Result != abi.Void {
		fmt.Fprint(c, "return ")
		if resultC == "void *" {
			fmt.Fprint(c, "(void *)")
		}
	}
	fmt.Fprintf(c, "((%s_function)address)(%s);\n}\n", helper, strings.Join(callArgs, ","))
	params := strings.Join(goParams, ",")
	call := fmt.Sprintf("C.%s(%s)", helper, strings.Join(goArgs, ","))
	convert := strings.Join(conversions, "")
	if s.Result == abi.Void {
		fmt.Fprintf(goCode, "\nfunc (b *%s) %s(%s) error {\nif b == nil { return dylib.ErrClosed }\nreturn b.symbol%d.WithAddress(func(address uintptr) error {\n%s%s\nreturn nil\n})\n}\n", bindingType, method, params, index, convert, call)
	} else {
		value := resultGo + "(" + call + ")"
		if s.Result == abi.Bool {
			value = call + " != 0"
		}
		fmt.Fprintf(goCode, "\nfunc (b *%s) %s(%s) (result %s,err error) {\nif b == nil { return result,dylib.ErrClosed }\nerr = b.symbol%d.WithAddress(func(address uintptr) error {\n%sresult = %s\nreturn nil\n})\nreturn result,err\n}\n", bindingType, method, params, resultGo, index, convert, value)
	}
	return nil
}

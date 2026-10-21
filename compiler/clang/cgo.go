package clang

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/ramblelabs/dylib-go/abi"
)

// CgoSource emits typed C bindings using fixed cgo bridges, without libffi.
// Go and llgo can compile the generated source. Scalars, opaque native pointers,
// ordinary/explicitly packed or aligned records, bitfields and function-pointer
// addresses are supported. Attributed values cross the Go boundary as
// ordinary transport records; C constructs and calls the native type. WithTail
// specifies a concrete variadic call shape; C performs default promotions.
// AMD64 supports fixed ms_abi/sysv_abi prototypes. Windows 386 also supports
// fixed stdcall/fastcall/thiscall prototypes. Fixed vectorcall
// prototypes on Windows amd64/386 require a supporting C compiler. Struct pointees
// use opaque native addresses; no Go struct pointer conversion is inferred.
// The caller owns the Session, native storage and function-pointer lifetimes.
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
	direct, err := newDirectTypes(h.Records, bindingType)
	if err != nil {
		return nil, err
	}
	mapper := &cgoTypes{
		directTypes: direct, cPrefix: prefix, pointerSize: h.Target.PointerSize,
		windows: h.Target.OS == "windows", msLayout: h.Target.msLayout(),
		ms386:      h.Target.msLayout() && h.Target.Arch == "386",
		gnuPacking: h.Target.gnuPacking(), gnuAMD64: h.Target.gnuPacking() && h.Target.Arch == "amd64",
		darwinARM64:      h.Target.OS == "darwin" && h.Target.Arch == "arm64",
		pointerTransport: h.Target.Arch == "arm64" || h.Target.gnuPacking() && h.Target.Arch == "amd64",
	}
	var preamble, methods bytes.Buffer
	fmt.Fprint(&preamble, "/*\n#include <stdint.h>\n")
	if len(h.Records) != 0 {
		fmt.Fprint(&preamble, "#include <stddef.h>\n")
		if err := mapper.writeCRecords(&preamble); err != nil {
			return nil, err
		}
		mapper.writeArgumentTypes(&preamble)
	}
	seen := make(map[string]bool)
	needsUnsafe := false
	for _, record := range h.Records {
		needsUnsafe = needsUnsafe || containsNativePointer(record.Description)
	}
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
		if err := h.writeCgoBridge(&preamble, &methods, function, bindingType, name, helper, i, mapper); err != nil {
			return nil, fmt.Errorf("clang: %s: %w", function.Name, err)
		}
	}
	if bytes.Contains(preamble.Bytes(), []byte("DYLIB_GO_VECTORCALL ")) {
		// An ignored calling-convention attribute would silently call the wrong ABI.
		// Keep ordinary generated sources unchanged; only vectorcall uses this token.
		guard := `#if defined(__has_attribute)
#if __has_attribute(vectorcall) && defined(_WIN32) && (defined(__i386__) || defined(__x86_64__))
#define DYLIB_GO_VECTORCALL __attribute__((vectorcall))
#endif
#endif
#ifndef DYLIB_GO_VECTORCALL
#error dylib_go_vectorcall_requires_a_supporting_C_compiler
#endif
`
		guarded := bytes.Replace(preamble.Bytes(), []byte("#include <stdint.h>\n"), []byte("#include <stdint.h>\n"+guard), 1)
		preamble.Reset()
		preamble.Write(guarded)
	}
	fmt.Fprint(&preamble, "*/\nimport \"C\"\n\n")
	// A separate C import keeps the preamble adjacent to cgo's special import.
	fmt.Fprint(&preamble, "import (\ndylib \"github.com/ramblelabs/dylib-go\"\n")
	if needsUnsafe {
		fmt.Fprint(&preamble, "\"unsafe\"\n")
	}
	if len(h.Records) != 0 {
		fmt.Fprint(&preamble, "\"fmt\"\n")
	}
	source = bytes.Replace(source, []byte("\nimport ("), preamble.Bytes(), 1)
	var output bytes.Buffer
	output.Write(source)
	mapper.writeRecords(&output)
	if err := mapper.writeConversions(&output); err != nil {
		return nil, err
	}
	fmt.Fprintf(&output, "\n// %s retains symbol handles; its caller owns the Session.\ntype %s struct {\n", bindingType, bindingType)
	for i := range h.Functions {
		fmt.Fprintf(&output, "symbol%d *dylib.Symbol\n", i)
	}
	fmt.Fprintf(&output, "}\n\n// New%s validates the host and resolves each requested export.\nfunc New%s(session *dylib.Session) (*%s,error) {\nif session == nil { return nil,dylib.ErrClosed }\nif err := %s.Target.CheckHost(); err != nil { return nil,err }\nb := &%s{}\n", bindingType, bindingType, bindingType, metadata, bindingType)
	mapper.writeCLayoutChecks(&output)
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
		return "", fmt.Errorf("cgo scalar conversion requires a scalar or opaque native pointer")
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

func (h *Header) cgoAttribute(s abi.Signature) (string, error) {
	if s.Convention == abi.Default || s.Convention == abi.CDecl {
		if h.Target.OS == "windows" && h.Target.Arch == "386" {
			return "__attribute__((cdecl)) ", nil
		}
		return "", nil
	}
	if h.Target.Arch == "amd64" && !s.Variadic {
		if s.Convention == abi.Win64 {
			return "__attribute__((ms_abi)) ", nil
		}
		if s.Convention == abi.SysV64 {
			return "__attribute__((sysv_abi)) ", nil
		}
	}

	if s.Convention == abi.VectorCall && h.Target.OS == "windows" && (h.Target.Arch == "amd64" || h.Target.Arch == "386") && !s.Variadic {
		if h.Target.Arch == "386" && h.Target.msLayout() {
			if s.Result == abi.Struct {
				return "", fmt.Errorf("MSVC i386 vectorcall record values require a separate compiler ABI adapter")
			}
			for _, arg := range s.Args {
				if arg == abi.Struct {
					return "", fmt.Errorf("MSVC i386 vectorcall record values require a separate compiler ABI adapter")
				}
			}
		}
		return "DYLIB_GO_VECTORCALL ", nil
	}
	if h.Target.OS == "windows" && h.Target.Arch == "386" && !s.Variadic {
		switch s.Convention {
		case abi.StdCall:
			return "__attribute__((stdcall)) ", nil
		case abi.FastCall:
			return "__attribute__((fastcall)) ", nil
		case abi.ThisCall:
			return "__attribute__((thiscall)) ", nil
		}
	}
	return "", fmt.Errorf("cgo bridges require cdecl, fixed amd64 ms_abi/sysv_abi, Windows 386 stdcall/fastcall/thiscall or Windows amd64/386 vectorcall prototypes")
}

func (h *Header) cgoPrototype(s abi.Signature, mapper *cgoTypes) (string, string, error) {
	if _, err := h.cgoAttribute(s); err != nil {
		return "", "", err
	}
	result, err := mapper.cType(s.ReturnType())
	if err != nil {
		return "", "", err
	}
	count := len(s.Args)
	if s.Variadic {
		count = s.FixedArgs
	}
	var parameters []string
	for i := 0; i < count; i++ {
		kind, err := mapper.cParameterType(s.ArgumentType(i))
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

func (h *Header) writeCgoBridge(c, goCode *bytes.Buffer, function Declaration, bindingType, method, helper string, index int, mapper *cgoTypes) error {
	s := function.Signature
	resultC, _, err := h.cgoPrototype(s, mapper)
	if err != nil {
		return err
	}
	resultGo, _ := mapper.goType(s.ReturnType())
	// Native entries retain their convention independently of the fixed cgo bridge.
	attribute, _ := h.cgoAttribute(s)
	boundaryC := make(map[int]string)
	for _, pointer := range function.FunctionPointers {
		name := fmt.Sprintf("%s_pointer%d", helper, pointer.Position+1)
		kind, err := h.writeCgoFunctionPointer(c, pointer, name, mapper)
		if err != nil {
			return fmt.Errorf("function pointer %d: %w", pointer.Position, err)
		}
		boundaryC[pointer.Position] = kind
	}
	var prototype, bridge, callArgs, goParams, goArgs, conversions, nativeLocals []string
	bridge = append(bridge, "uintptr_t address")
	goArgs = append(goArgs, "C.uintptr_t(address)")
	count := len(s.Args)
	if s.Variadic {
		count = s.FixedArgs
	}
	for i := range s.Args {
		kind, err := mapper.cType(s.ArgumentType(i))
		if err != nil {
			return fmt.Errorf("argument %d: %w", i, err)
		}
		goKind, _ := mapper.goType(s.ArgumentType(i))
		goParams = append(goParams, fmt.Sprintf("p%d %s", i, goKind))
		transport := mapper.transport(s.ArgumentType(i))
		indirectTransport := mapper.pointerTransport && s.Args[i] == abi.Struct
		if indirectTransport {
			// Keep aggregate register allocation and stack layout inside C,
			// including SysV and AArch64 values after argument registers run out.
			bridge = append(bridge, fmt.Sprintf("const %s *p%d", transport, i))
		} else {
			bridge = append(bridge, fmt.Sprintf("%s p%d", transport, i))
		}
		argument := fmt.Sprintf("p%d", i)
		if s.Args[i] == abi.Struct {
			index, _ := mapper.recordIndex(s.ArgumentType(i))
			converted := mapper.conversion(index, true) + "(" + argument + ")"
			if indirectTransport {
				conversions = append(conversions, fmt.Sprintf("c%d := %s\n", i, converted))
				goArgs = append(goArgs, fmt.Sprintf("&c%d", i))
				argument = "*" + argument
			} else {
				goArgs = append(goArgs, converted)
			}
		} else if s.Args[i] == abi.Bool {
			conversions = append(conversions, fmt.Sprintf("var c%d C.int32_t\nif p%d { c%d = 1 }\n", i, i, i))
			goArgs = append(goArgs, fmt.Sprintf("c%d", i))
		} else if s.Args[i] != abi.Pointer {
			goArgs = append(goArgs, fmt.Sprintf("C.%s(p%d)", transport, i))
		} else {
			goArgs = append(goArgs, argument)
		}
		if s.Args[i] == abi.Struct && transport != kind {
			index, _ := mapper.recordIndex(s.ArgumentType(i))
			argument = mapper.cConversion(index, true) + "(" + argument + ")"
		} else if transport != kind {
			argument = "(" + kind + ")" + argument
		}
		if callback, ok := boundaryC[i]; ok {
			kind = callback
			argument = "(" + callback + ")" + argument
		}
		if record, ok := mapper.indirectArgument(s.ArgumentType(i)); ok && i < count {
			local := fmt.Sprintf("native%d", i)
			nativeLocals = append(nativeLocals, fmt.Sprintf("%s %s=%s;\n", kind, local, argument))
			kind, _ = mapper.cParameterType(s.ArgumentType(i))
			argument = mapper.cRecordName(record) + "_arg(" + local + ")"
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
	mapper.writeNativePrototype(c, s, helper+"_function", prototypeResult, attribute, prototype)
	fmt.Fprintf(c, "static %s %s(%s) {\n", mapper.transport(s.ReturnType()), helper, strings.Join(bridge, ","))
	fmt.Fprint(c, strings.Join(nativeLocals, ""))
	resultIndex, physicalResult := mapper.registerResult(s)
	guardedResult := physicalResult && s.Convention != abi.ThisCall
	if physicalResult {
		i := resultIndex
		name := mapper.cRecordName(i)
		var arguments []string
		for _, position := range mapper.registerResultOrder(s) {
			argument := fmt.Sprintf("(%s_result_pointer){&nativeResult}", name)
			if s.Convention == abi.ThisCall {
				argument = "&nativeResult"
			}
			if position >= 0 {
				argument = callArgs[position]
			}
			arguments = append(arguments, argument)
		}
		if guardedResult {
			fmt.Fprintf(c, "#if %s\n", gccRegisterResult)
		}
		fmt.Fprintf(c, "%s nativeResult={0};\n((%s_function)address)(%s);\nreturn %s(nativeResult);\n", name, helper, strings.Join(arguments, ","), mapper.cConversion(i, false))
		if guardedResult {
			fmt.Fprint(c, "#else\n")
		}
	}
	if !physicalResult || guardedResult {
		if s.Result != abi.Void {
			fmt.Fprint(c, "return ")
			if s.Result == abi.Pointer {
				fmt.Fprint(c, "(void *)")
			}
		}
		nativeCall := fmt.Sprintf("((%s_function)address)(%s)", helper, strings.Join(callArgs, ","))
		if s.Result == abi.Struct {
			index, _ := mapper.recordIndex(s.ReturnType())
			if mapper.needsTransport(index) {
				nativeCall = mapper.cConversion(index, false) + "(" + nativeCall + ")"
			}
		}
		fmt.Fprintf(c, "%s;\n", nativeCall)
	}
	if guardedResult {
		fmt.Fprint(c, "#endif\n")
	}
	fmt.Fprint(c, "}\n")
	params := strings.Join(goParams, ",")
	call := fmt.Sprintf("C.%s(%s)", helper, strings.Join(goArgs, ","))
	convert := strings.Join(conversions, "")
	if s.Result == abi.Void {
		fmt.Fprintf(goCode, "\nfunc (b *%s) %s(%s) error {\nif b == nil { return dylib.ErrClosed }\nreturn b.symbol%d.WithAddress(func(address uintptr) error {\n%s%s\nreturn nil\n})\n}\n", bindingType, method, params, index, convert, call)
	} else {
		value := resultGo + "(" + call + ")"
		if s.Result == abi.Struct {
			index, _ := mapper.recordIndex(s.ReturnType())
			value = mapper.conversion(index, false) + "(" + call + ")"
		}
		if s.Result == abi.Bool {
			value = call + " != 0"
		}
		fmt.Fprintf(goCode, "\nfunc (b *%s) %s(%s) (result %s,err error) {\nif b == nil { return result,dylib.ErrClosed }\nerr = b.symbol%d.WithAddress(func(address uintptr) error {\n%sresult = %s\nreturn nil\n})\nreturn result,err\n}\n", bindingType, method, params, resultGo, index, convert, value)
	}
	return nil
}

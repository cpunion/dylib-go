package clang

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/cpunion/dylib-go/abi"
)

// MSVC i386 can pass explicitly aligned aggregates through a hidden pointer;
// MinGW's ordinary C prototype passes inline bytes instead. Observe the producer
// lowering rather than deriving required alignment from storage alignment.
func (h *Header) probeAlignedArguments(ctx context.Context, compiler, path string, flags []string) error {
	if h.Target.OS != "windows" || h.Target.Arch != "386" || !strings.Contains(h.Target.Triple, "msvc") {
		return nil
	}
	var source strings.Builder
	var indexes []int
	for i, record := range h.Records {
		if record.Layout.Alignment <= 4 {
			continue
		}
		indexes = append(indexes, i)
		fmt.Fprintf(&source, "void dylib_record_%d_argument(%s value) {}\n", i, record.Name)
		fmt.Fprintf(&source, "%s dylib_record_%d_result(void) { %s value={0}; return value; }\n", record.Name, i, record.Name)
	}
	if len(indexes) == 0 {
		return nil
	}
	args := append(append([]string(nil), flags...), "-x", "c", "-std=c17", "-include", path, "-O0", "-S", "-emit-llvm", "-o", "-", "-")
	data, err := runCompiler(ctx, compiler, args, source.String(), maxASTSize)
	if err != nil {
		return err
	}
	for _, i := range indexes {
		indirect, err := indirectRecordArgument(string(data), fmt.Sprintf("dylib_record_%d_argument", i))
		if err != nil {
			return err
		}
		h.Records[i].IndirectArgument = indirect
		if indirect {
			parameters, err := recordProbeParameters(string(data), fmt.Sprintf("dylib_record_%d_result", i))
			if err != nil {
				return err
			}
			h.Records[i].IndirectResult = strings.Contains(parameters, "sret(")
		}
	}
	return nil
}

func indirectRecordArgument(ir, name string) (bool, error) {
	parameter, err := recordProbeParameters(ir, name)
	if err != nil {
		return false, err
	}
	pointer := strings.HasPrefix(parameter, "ptr ") || strings.Contains(parameter, "*")
	return pointer && !strings.Contains(parameter, "byval(") && !strings.Contains(parameter, ","), nil
}

func recordProbeParameters(ir, name string) (string, error) {
	marker := "@" + name + "("
	for _, line := range strings.Split(ir, "\n") {
		if !strings.HasPrefix(line, "define ") {
			continue
		}
		_, parameters, found := strings.Cut(line, marker)
		if !found {
			continue
		}
		depth := 0
		for i, ch := range parameters {
			if ch == '(' {
				depth++
			}
			if ch != ')' {
				continue
			}
			if depth != 0 {
				depth--
				continue
			}
			return parameters[:i], nil
		}
	}
	return "", fmt.Errorf("clang: missing compiler ABI probe %s", name)
}

func (t *cgoTypes) indirectArgument(d abi.TypeDesc) (int, bool) {
	if d.Type != abi.Struct {
		return 0, false
	}
	i, err := t.recordIndex(d)
	if err != nil {
		return 0, false
	}
	return i, t.records[i].IndirectArgument
}

func (t *cgoTypes) cParameterType(d abi.TypeDesc) (string, error) {
	if index, ok := t.indirectArgument(d); ok {
		return t.cRecordName(index) + "_argument", nil
	}
	return t.cType(d)
}

func (t *cgoTypes) writeArgumentTypes(output *bytes.Buffer) {
	for i, record := range t.records {
		if !record.IndirectArgument {
			continue
		}
		name := t.cRecordName(i)
		fmt.Fprintf(output, "#if defined(_WIN32) && defined(__i386__) && !defined(_MSC_VER)\ntypedef %s *%s_argument;\n#define %s_arg(value) (&(value))\n#else\ntypedef %s %s_argument;\n#define %s_arg(value) (value)\n#endif\n", name, name, name, name, name, name)
		if record.IndirectResult {
			fmt.Fprintf(output, "typedef struct { %s *value; } %s_result_pointer;\n", name, name)
		}
	}
}

const gccFastResult = "defined(_WIN32) && defined(__i386__) && defined(__GNUC__) && !defined(__clang__)"

func (t *cgoTypes) fastResult(s abi.Signature) (int, bool) {
	if s.Convention != abi.FastCall || s.Result != abi.Struct {
		return 0, false
	}
	i, err := t.recordIndex(s.ReturnType())
	return i, err == nil && t.records[i].IndirectResult
}

// GCC puts a fastcall sret pointer in ECX; MSVC/Clang leave it on the stack.
// Put register arguments first, then a one-pointer aggregate for the hidden
// stack result, then the original stack arguments. GCC aggregates consume
// register slots even though they are passed on the stack.
func (t *cgoTypes) fastResultOrder(s abi.Signature) []int {
	var registers, stack []int
	for i := range s.Args {
		kind := s.Args[i]
		_, indirect := t.indirectArgument(s.ArgumentType(i))
		eligible := indirect || kind == abi.Pointer || kind == abi.Bool || kind == abi.I8 || kind == abi.U8 || kind == abi.I16 || kind == abi.U16 || kind == abi.I32 || kind == abi.U32
		if eligible && len(registers) < 2 {
			registers = append(registers, i)
		} else {
			stack = append(stack, i)
		}
	}
	return append(append(registers, -1), stack...)
}

func (t *cgoTypes) writeNativePrototype(output *bytes.Buffer, s abi.Signature, name, result, attribute string, parameters []string) {
	if i, ok := t.fastResult(s); ok {
		var physical []string
		for _, position := range t.fastResultOrder(s) {
			kind := t.cRecordName(i) + "_result_pointer"
			if position >= 0 {
				kind = parameters[position]
			}
			physical = append(physical, kind)
		}
		fmt.Fprintf(output, "#if %s\ntypedef void (%s*%s)(%s);\n#else\n", gccFastResult, attribute, name, strings.Join(physical, ","))
	}
	fmt.Fprintf(output, "typedef %s (%s*%s)(%s);\n", result, attribute, name, strings.Join(parameters, ","))
	if _, ok := t.fastResult(s); ok {
		fmt.Fprint(output, "#endif\n")
	}
}

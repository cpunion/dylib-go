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
	}
	return nil
}

func indirectRecordArgument(ir, name string) (bool, error) {
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
			parameter := parameters[:i]
			pointer := strings.HasPrefix(parameter, "ptr ") || strings.Contains(parameter, "*")
			return pointer && !strings.Contains(parameter, "byval(") && !strings.Contains(parameter, ","), nil
		}
	}
	return false, fmt.Errorf("clang: missing compiler argument probe %s", name)
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
	}
}

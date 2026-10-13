package clang

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var errTypedefABI = errors.New("clang: aligned typedef records require Clang 19-compatible SysV lowering")

// Clang 18 classified lowered scalar aliases differently from canonical scalar
// fields on SysV AMD64. Some floating aggregates were even truncated. Check
// effective lowering with the requested flags, not the compiler version string.
func (h *Header) probeTypedefABI(ctx context.Context, compiler string, flags []string) error {
	if h.Target.Arch != "amd64" || !h.Target.gnuPacking() {
		return nil
	}
	needed := false
	for _, record := range h.Records {
		needed = needed || len(record.FieldTypeAlignments) != 0
	}
	if !needed {
		return nil
	}
	const source = `
#pragma pack(push,0)
typedef int dylib_typedef_abi_int __attribute__((aligned(1)));
typedef struct { signed char tag; dylib_typedef_abi_int value; } dylib_typedef_abi_record;
dylib_typedef_abi_record dylib_typedef_abi_probe(dylib_typedef_abi_record value) { return value; }
#pragma pack(pop)
`
	args := append(append([]string(nil), flags...), "-x", "c", "-std=c17", "-O0", "-S", "-emit-llvm", "-o", "-", "-")
	data, err := runCompiler(ctx, compiler, args, source, maxASTSize)
	if err != nil {
		return err
	}
	parameters, err := recordProbeParameters(string(data), "dylib_typedef_abi_probe")
	if err != nil {
		return err
	}
	if !strings.Contains(parameters, "sret(") || !strings.Contains(parameters, "byval(") {
		return fmt.Errorf("%w: compiler emitted %s", errTypedefABI, parameters)
	}
	return nil
}

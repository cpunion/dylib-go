package clang

import (
	"context"
	"fmt"
	"strings"
)

// AAPCS64 distinguishes natural 16-byte member alignment from a record aligned
// only by an attribute. Observe Clang's argument coercion instead of inferring
// even-register allocation from the record size/alignment alone.
func (h *Header) probeBitfieldArgumentAlignment(ctx context.Context, compiler, path string, flags []string) error {
	var source strings.Builder
	var indexes []int
	for i, r := range h.Records {
		if !r.hasBitfieldTypedefs() || r.Layout.Size > 16 || r.Layout.Alignment < 16 {
			continue
		}
		indexes = append(indexes, i)
		fmt.Fprintf(&source, "void dylib_record_%d_argument(%s value) {}\n", i, r.Name)
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
		parameter, err := recordProbeParameters(string(data), fmt.Sprintf("dylib_record_%d_argument", i))
		if err != nil {
			return err
		}
		if strings.HasPrefix(parameter, "i128 ") {
			h.Records[i].ArgumentAlignment = 16
		}
	}
	return nil
}

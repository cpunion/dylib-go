package clang

import (
	"context"
	"fmt"

	"github.com/ramblelabs/dylib-go/compiler/internal/clangcmd"
)

func runCompiler(ctx context.Context, compiler string, args []string, input string, limit int) ([]byte, error) {
	return runCompilerStream(ctx, compiler, args, input, limit, false)
}

func runCompilerStream(ctx context.Context, compiler string, args []string, input string, limit int, diagnostics bool) ([]byte, error) {
	data, err := clangcmd.Run(ctx, compiler, args, input, limit, diagnostics)
	if err != nil && err != ctx.Err() {
		err = fmt.Errorf("clang: %w", err)
	}
	return data, err
}

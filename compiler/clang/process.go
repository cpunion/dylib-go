package clang

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type boundedOutput struct {
	buffer   bytes.Buffer
	limit    int
	cancel   context.CancelFunc
	exceeded bool
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	n := len(data)
	if remaining := b.limit - b.buffer.Len(); n > remaining {
		data, b.exceeded = data[:remaining], true
	}
	b.buffer.Write(data)
	if b.exceeded && b.cancel != nil {
		b.cancel()
	}
	return n, nil
}

func runCompiler(ctx context.Context, compiler string, args []string, input string, limit int) ([]byte, error) {
	return runCompilerStream(ctx, compiler, args, input, limit, false)
}

// Driver inspection prints its frontend command on stderr. Share process
// cancellation and output budgets with the AST/IR invocations.
func runCompilerStream(ctx context.Context, compiler string, args []string, input string, limit int, diagnostics bool) ([]byte, error) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	stdout := boundedOutput{limit: limit, cancel: cancel}
	stderr := boundedOutput{limit: maxDiagnostics}
	if diagnostics {
		stdout.limit = maxDiagnostics
		stderr.limit, stderr.cancel = limit, cancel
	}
	cmd := exec.CommandContext(child, compiler, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(input), &stdout, &stderr
	err := cmd.Run()
	if stdout.exceeded || diagnostics && stderr.exceeded {
		return nil, fmt.Errorf("clang: compiler output exceeds %d bytes", limit)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		diagnostics := stderr.buffer.String()
		if stderr.exceeded {
			diagnostics += "\n[diagnostics truncated]"
		}
		return nil, fmt.Errorf("clang: %s: %w\n%s", compiler, err, diagnostics)
	}
	if diagnostics {
		return stderr.buffer.Bytes(), nil
	}
	return stdout.buffer.Bytes(), nil
}

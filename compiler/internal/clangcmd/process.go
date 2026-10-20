// Package clangcmd runs optional compiler probes with bounded output and cancellation.
package clangcmd

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

// DiagnosticsLimit bounds diagnostic storage for optional compiler probes.
const DiagnosticsLimit = 32 << 10

// Run bounds stdout and diagnostic storage. Diagnostics selects stderr as the
// result stream for driver inspection; errors retain bounded compiler diagnostics.
func Run(ctx context.Context, compiler string, args []string, input string, limit int, diagnostics bool) ([]byte, error) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	stdout := boundedOutput{limit: limit, cancel: cancel}
	stderr := boundedOutput{limit: DiagnosticsLimit}
	if diagnostics {
		stdout.limit = DiagnosticsLimit
		stderr.limit, stderr.cancel = limit, cancel
	}
	cmd := exec.CommandContext(child, compiler, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(input), &stdout, &stderr
	err := cmd.Run()
	if stdout.exceeded || diagnostics && stderr.exceeded {
		return nil, fmt.Errorf("compiler output exceeds %d bytes", limit)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		diagnostics := stderr.buffer.String()
		if stderr.exceeded {
			diagnostics += "\n[diagnostics truncated]"
		}
		// Build drivers such as SwiftPM also report compiler failures on stdout.
		// Keep its tail so an unsuccessful build retains the final diagnostic.
		output := stdout.buffer.Bytes()
		if len(output) > DiagnosticsLimit {
			output = output[len(output)-DiagnosticsLimit:]
			diagnostics += "\n[stdout truncated]"
		}
		if len(output) != 0 {
			diagnostics += "\n" + string(output)
		}
		return nil, fmt.Errorf("%s: %w\n%s", compiler, err, diagnostics)
	}
	if diagnostics {
		return stderr.buffer.Bytes(), nil
	}
	return stdout.buffer.Bytes(), nil
}

// Package llvm optionally compiles LLVM IR/bitcode to native object files using
// the external llc tool. It does not add an LLVM dependency to the loader.
package llvm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	dylib "github.com/cpunion/dylib-go"
)

const maxInputSize = 256 << 20
const maxDiagnosticSize = 32 << 10

// Options selects the installed LLVM compiler and temporary storage. LLVM's
// reader must support the producer's IR/bitcode version. No target override is
// passed: llc preserves the module target and data layout. Target-less modules
// use llc's default target; Session.Load still checks host compatibility.
type Options struct {
	Compiler string // llc executable, default "llc"; never a shell command.
	TempDir  string // Parent for owned temporary files, default OS temporary dir.
}

// Object owns a compiled object file. Load Path into a session before Close;
// the session snapshots native object bytes and can outlive this artifact.
// Keep directory-dependent imports available through Session.LibraryPaths.
// Path must not be read concurrently with Close. Do not copy Object.
type Object struct {
	Path string
	Info dylib.Info
	mu   sync.Mutex
	dir  string
}

// Close deletes compiler input/output storage. It does not unload a session
// that has already loaded the object. Close is idempotent and retries failures.
func (o *Object) Close() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.dir == "" {
		return nil
	}
	if err := os.RemoveAll(o.dir); err != nil {
		return err
	}
	o.dir = ""
	return nil
}

// Compile snapshots a regular IR/bitcode file and emits PIC native object code.
// It does not infer C signatures, link language runtimes, or execute the input.
// The normal loader's target, relocation and runtime checks still apply.
func Compile(ctx context.Context, path string, opts Options) (_ *Object, err error) {
	if ctx == nil {
		return nil, fmt.Errorf("llvm: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	stat, err := source.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > maxInputSize {
		return nil, fmt.Errorf("llvm: expected regular input of at most %d bytes", maxInputSize)
	}
	dir, err := os.MkdirTemp(opts.TempDir, "dylib-go-llvm-")
	if err != nil {
		return nil, err
	}
	object := &Object{dir: dir, Path: filepath.Join(dir, "output.o")}
	defer func() {
		if err != nil {
			if cleanup := object.Close(); cleanup != nil {
				err = errors.Join(err, fmt.Errorf("llvm: temporary cleanup: %w", cleanup))
			}
		}
	}()
	input := filepath.Join(dir, "input")
	snapshot, err := os.Create(input)
	if err != nil {
		return nil, err
	}
	size, copyErr := io.Copy(snapshot, io.LimitReader(source, maxInputSize+1))
	closeErr := snapshot.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if size > maxInputSize {
		return nil, fmt.Errorf("llvm: input grew beyond %d bytes", maxInputSize)
	}
	tool := opts.Compiler
	if tool == "" {
		tool = "llc"
	}
	cmd := exec.CommandContext(ctx, tool, "-filetype=obj", "-relocation-model=pic", "-o", object.Path, input)
	var diagnostics limitedDiagnostics
	cmd.Stdout, cmd.Stderr = &diagnostics, &diagnostics
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("llvm: %s: %w\n%s", tool, err, diagnostics.String())
	}
	object.Info, err = dylib.Inspect(object.Path)
	if err != nil {
		return nil, fmt.Errorf("llvm: invalid compiler output: %w", err)
	}
	if object.Info.Kind != "object" {
		return nil, fmt.Errorf("llvm: compiler output is %s, expected object", object.Info.Kind)
	}
	return object, nil
}

type limitedDiagnostics struct {
	bytes.Buffer
	truncated bool
}

func (d *limitedDiagnostics) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := maxDiagnosticSize - d.Len(); n > remaining {
		p, d.truncated = p[:remaining], true
	}
	d.Buffer.Write(p)
	return n, nil
}

func (d *limitedDiagnostics) String() string {
	if d.truncated {
		return d.Buffer.String() + "\n[diagnostics truncated]"
	}
	return d.Buffer.String()
}

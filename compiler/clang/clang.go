// Package clang extracts explicit C declarations with an optional external Clang
// compiler. It does not infer signatures from object symbols or link runtimes.
package clang

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/cpunion/dylib-go/abi"
)

// Options selects a C compiler, target, preprocessing flags and explicit names.
// Target defaults to the qualified native host target. Flags must match those
// used to build the exports; use Target rather than a target override in Flags.
type Options struct {
	Compiler  string // Clang executable, default clang; never a shell command.
	Target    string
	Flags     []string
	Functions []string
}

type Target struct {
	Triple      string
	OS, Arch    string
	PointerSize int
}

// Declaration describes a C name, the loader's object symbol spelling and a
// fixed signature or variadic prefix. It does not establish that an image exports
// the symbol, or describe native pointer ownership.
type Declaration struct {
	Name, Symbol string
	Signature    abi.Signature
}

type Header struct {
	Target    Target
	Functions []Declaration
}

// Lookup returns an independent declaration snapshot, including its signature.
func (h *Header) Lookup(name string) (Declaration, error) {
	if h != nil {
		for _, function := range h.Functions {
			if function.Name == name {
				if err := function.Signature.Validate(); err != nil {
					return Declaration{}, err
				}
				function.Signature = function.Signature.Clone()
				return function, nil
			}
		}
	}
	return Declaration{}, fmt.Errorf("clang: no declaration for %q", name)
}

// ForHost rejects foreign target metadata before returning a call declaration.
// The loaded export must still match the header and preprocessing flags.
func (h *Header) ForHost(name string) (Declaration, error) {
	if h == nil {
		return Declaration{}, fmt.Errorf("clang: nil header")
	}
	target, err := parseTarget(h.Target.Triple, h.Target.PointerSize)
	if err != nil || target != h.Target || target.OS != runtime.GOOS || target.Arch != runtime.GOARCH || target.PointerSize != strconv.IntSize/8 {
		return Declaration{}, fmt.Errorf("clang: declaration target does not match %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return h.Lookup(name)
}

// WithTail adds concrete variadic argument descriptors to a declaration prefix.
// Ordinary declarations reject tails. abi.Prepare performs C default promotions.
func (d Declaration) WithTail(types ...abi.Type) (Declaration, error) {
	if err := d.Signature.Validate(); err != nil {
		return Declaration{}, err
	}
	d.Signature = d.Signature.Clone()
	if !d.Signature.Variadic || len(d.Signature.Args) != d.Signature.FixedArgs {
		return Declaration{}, fmt.Errorf("clang: expected an unexpanded variadic prefix")
	}
	d.Signature.Args = append(d.Signature.Args, types...)
	if err := d.Signature.Validate(); err != nil {
		return Declaration{}, err
	}
	return d, nil
}

const maxHeaderSize = 16 << 20
const maxASTSize = 32 << 20
const maxDiagnostics = 32 << 10
const maxFunctions = 256

// Parse asks Clang to preprocess a C17 header and evaluate primitive size probes.
// It extracts only requested external prototypes, supported scalars/typedefs and
// opaque pointers. Aggregates, enums, function pointers, inline/static functions
// and unsupported calling conventions are rejected. Headers and their includes
// must stay stable during parsing; they are read by Clang at their original paths.
func Parse(ctx context.Context, path string, opts Options) (*Header, error) {
	if ctx == nil {
		return nil, fmt.Errorf("clang: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts.Functions = append([]string(nil), opts.Functions...)
	if len(opts.Functions) == 0 || len(opts.Functions) > maxFunctions {
		return nil, fmt.Errorf("clang: request 1-%d explicit function names", maxFunctions)
	}
	seen := make(map[string]bool)
	for _, name := range opts.Functions {
		if !cIdentifier(name) || seen[name] {
			return nil, fmt.Errorf("clang: invalid or duplicate function name %q", name)
		}
		seen[name] = true
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > maxHeaderSize {
		return nil, fmt.Errorf("clang: expected regular header of at most %d bytes", maxHeaderSize)
	}
	compiler := opts.Compiler
	if compiler == "" {
		compiler = "clang"
	}
	compiler, err = exec.LookPath(compiler)
	if err != nil {
		return nil, err
	}
	target := opts.Target
	if target == "" {
		target = nativeTriple()
		if target == "" {
			return nil, fmt.Errorf("clang: unqualified host; specify a target for inspection")
		}
	}
	flags := append(append([]string(nil), opts.Flags...), "--target="+target)
	triple, err := runCompiler(ctx, compiler, append(append([]string(nil), flags...), "-dumpmachine"), "", 4096)
	if err != nil {
		return nil, err
	}
	args := append(flags, "-x", "c", "-std=c17", "-include", path, "-Xclang", "-ast-dump=json", "-fsyntax-only", "-")
	data, err := runCompiler(ctx, compiler, args, sizeProbe, maxASTSize)
	if err != nil {
		return nil, err
	}
	header, err := decodeDeclarations(data, string(triple), opts.Functions)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return header, nil
}

func nativeTriple() string {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH]
	if arch == "" {
		return ""
	}
	switch runtime.GOOS {
	case "linux":
		return arch + "-unknown-linux-gnu"
	case "windows":
		return arch + "-pc-windows-msvc"
	case "darwin":
		if runtime.GOARCH == "386" {
			return ""
		}
		if arch == "aarch64" {
			arch = "arm64"
		}
		return arch + "-apple-macosx11"
	}
	return ""
}

func cIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(i != 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

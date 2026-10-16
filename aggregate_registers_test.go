//go:build cgo && (linux || darwin || windows)

package dylib

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Both public call paths use the same independently compiled producers, including
// optimized code, lazy archives and OS libraries. No compiler subprocess builds
// a Go caller: the active Go/llgo test compiler supplies it.
func withAggregateRegisters(t *testing.T, check func(*testing.T, *Session)) {
	t.Helper()
	needNative(t)
	for _, optimization := range []string{"-O0", "-O2"} {
		t.Run(optimization, func(t *testing.T) {
			dir := t.TempDir()
			source := "testdata/aggregate_registers.c"
			object := compile(t, source, filepath.Join(dir, "aggregate.o"), optimization)
			archive := filepath.Join(dir, "aggregate.a")
			command(t, "ar", "rcs", archive, object)
			library, flag := filepath.Join(dir, "aggregate.so"), "-shared"
			if runtime.GOOS == "darwin" {
				library, flag = filepath.Join(dir, "aggregate.dylib"), "-dynamiclib"
			} else if runtime.GOOS == "windows" {
				library = filepath.Join(dir, "aggregate.dll")
			}
			flags := []string{flag, optimization, source, "-o", library}
			if runtime.GOOS != "windows" {
				flags = append(flags, "-fPIC")
			}
			if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
				flags = append(flags, "--target=x86_64-apple-macosx11")
			}
			command(t, compiler(), flags...)
			for _, input := range []struct{ name, path string }{{"object", object}, {"archive", archive}, {"library", library}} {
				t.Run(input.name, func(t *testing.T) {
					s := New(Options{ProcessSymbols: runtime.GOOS != "windows"})
					defer func() {
						if err := s.Close(); err != nil {
							t.Error(err)
						}
					}()
					if runtime.GOOS == "windows" && input.name != "library" {
						directory := "System32"
						if runtime.GOARCH == "386" {
							directory = "SysWOW64"
						}
						load(t, s, filepath.Join(os.Getenv("SystemRoot"), directory, "msvcrt.dll"))
					}
					load(t, s, input.path)
					check(t, s)
				})
			}
		})
	}
}

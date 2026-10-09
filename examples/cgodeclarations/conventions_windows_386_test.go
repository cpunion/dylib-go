//go:build cgo

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/compiler/clang"
)

func TestGeneratedWindows386Conventions(t *testing.T) {
	compiler := os.Getenv("DYLIB_LLVM_CLANG")
	if compiler == "" {
		compiler = os.Getenv("CLANG")
	}
	if compiler == "" {
		compiler = "clang"
	}
	if _, err := exec.LookPath(compiler); err != nil {
		if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	names := []string{"std_add", "std_mixed", "fast_add", "fast_mixed", "std_factory", "fast_factory", "apply_std", "apply_fast", "std_apply_fast", "fast_apply_std"}
	header, err := clang.Parse(context.Background(), "testdata/conventions.h", clang.Options{Compiler: compiler, Target: _ConventionsDeclarations.Target.Triple, Functions: names})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := header.CgoSource("main", "Conventions")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("conventions_windows_386.go")
	if err != nil || !bytes.Equal(fresh, bytes.ReplaceAll(committed, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated conventions are stale:", err)
	}
	object := filepath.Join(t.TempDir(), "conventions.o")
	if data, err := exec.Command(compiler, "--target="+header.Target.Triple, "-O0", "-c", "-fno-stack-protector", "testdata/conventions.c", "-o", object).CombinedOutput(); err != nil {
		t.Fatalf("C convention producer: %v\n%s", err, data)
	}
	session := dylib.New(dylib.Options{})
	defer session.Close()
	if err := session.Load(object); err != nil {
		t.Fatal(err)
	}
	b, err := NewConventions(session)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		for _, add := range []func(int32, int32) (int32, error){b.Std_add, b.Fast_add} {
			if got, err := add(-20, 62); err != nil || got != 42 {
				t.Fatal("convention integer call:", got, err)
			}
		}
		if got, err := b.Std_mixed(10, 20.5, 1.5, 10); err != nil || got != 42 {
			t.Fatal("stdcall mixed call:", got, err)
		}
		if got, err := b.Fast_mixed(10, 10, 20.5, 1.5); err != nil || got != 42 {
			t.Fatal("fastcall mixed call:", got, err)
		}
	}
	factory, err := session.Resolve("std_factory")
	if err != nil {
		t.Fatal(err)
	}
	err = factory.WithAddress(func(_ uintptr) error {
		stdEntry, err := b.Std_factory()
		if err != nil || stdEntry == nil {
			t.Fatal("stdcall factory:", err)
		}
		fastEntry, err := b.Fast_factory()
		if err != nil || fastEntry == nil {
			t.Fatal("fastcall factory:", err)
		}
		for i := 0; i < 100; i++ {
			if got, err := b.Apply_std(stdEntry, 20, 22); err != nil || got != 42 {
				t.Fatal("cdecl to stdcall entry:", got, err)
			}
			if got, err := b.Apply_fast(fastEntry, 20, 22); err != nil || got != 42 {
				t.Fatal("cdecl to fastcall entry:", got, err)
			}
			if got, err := b.Std_apply_fast(fastEntry, 20, 22); err != nil || got != 42 {
				t.Fatal("stdcall to fastcall entry:", got, err)
			}
			if got, err := b.Fast_apply_std(stdEntry, 20, 22); err != nil || got != 42 {
				t.Fatal("fastcall to stdcall entry:", got, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Std_add(20, 22); !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed convention call:", err)
	}
	if entry, err := b.Fast_factory(); entry != nil || !errors.Is(err, dylib.ErrClosed) {
		t.Fatal("closed convention factory:", err)
	}
}

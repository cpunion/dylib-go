//go:build cgo && darwin && (amd64 || arm64)

package swiftcall

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	dylib "github.com/ramblelabs/dylib-go"
	"github.com/ramblelabs/dylib-go/abibridge"
	builder "github.com/ramblelabs/dylib-go/compiler/abibridge"
)

func TestNativeSwiftExample(t *testing.T) {
	facade := os.Getenv("DYLIB_ABIBRIDGE_FACADE")
	if facade == "" {
		source, ffi := os.Getenv("DYLIB_ABIBRIDGE_SOURCE"), os.Getenv("DYLIB_ABIBRIDGE_LIBFFI_SOURCE")
		if source == "" || ffi == "" {
			if os.Getenv("DYLIB_TEST_REQUIRE_ABIBRIDGE") == "1" {
				t.Fatal("required pinned ABIBridge sources missing")
			}
			t.Skip("optional ABIBridge facade/source not configured")
		}
		library, err := builder.Build(context.Background(), builder.Options{Source: source, LibffiSource: ffi})
		if err != nil {
			t.Fatal(err)
		}
		defer library.Close()
		facade = library.Path
	}
	r, err := abibridge.Open(facade)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	sdk, err := exec.Command("xcrun", "--sdk", "macosx", "--show-sdk-path").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, optimization := range []string{"-Onone", "-O"} {
		t.Run(optimization, func(t *testing.T) {
			library := filepath.Join(t.TempDir(), "addition.dylib")
			arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH]
			command := exec.Command("xcrun", "swiftc", "-sdk", strings.TrimSpace(string(sdk)), "-target", arch+"-apple-macosx15.4", optimization, "-emit-library", "testdata/add.swift", "-o", library)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
			s := dylib.New(dylib.Options{KeepLibraries: true})
			defer s.Close()
			if err := s.Load(library); err != nil {
				t.Fatal(err)
			}
			symbol, err := s.Resolve("fixture_example_add")
			if err != nil {
				t.Fatal(err)
			}
			if got, err := Add(r, symbol, 20, 22); err != nil || got != 42 {
				t.Fatal(got, err)
			}
		})
	}
}

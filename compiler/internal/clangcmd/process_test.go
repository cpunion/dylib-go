package clangcmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestFailedDriverDiagnostics(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"short", "long"} {
		t.Run(mode, func(t *testing.T) {
			_, err := Run(context.Background(), executable, []string{
				"-test.run=^TestDriverDiagnosticProcess$", "--", "clangcmd-helper", mode,
			}, "", 2*DiagnosticsLimit, false)
			if err == nil || !strings.Contains(err.Error(), "driver stderr") || !strings.Contains(err.Error(), "compiler failure on stdout") {
				t.Fatalf("missing failed-driver diagnostics: %v", err)
			}
			if mode == "long" && (!strings.Contains(err.Error(), "[stdout truncated]") || strings.Contains(err.Error(), "discarded stdout prefix")) {
				t.Fatal("large driver output did not retain only its bounded tail")
			}
			if len(err.Error()) > DiagnosticsLimit+1024 {
				t.Fatalf("diagnostic length = %d", len(err.Error()))
			}
		})
	}
}

func TestDriverDiagnosticProcess(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "clangcmd-helper" {
		return
	}
	if os.Args[len(os.Args)-1] == "long" {
		fmt.Print("discarded stdout prefix", strings.Repeat("x", DiagnosticsLimit))
	}
	fmt.Println("compiler failure on stdout")
	fmt.Fprintln(os.Stderr, "driver stderr")
	os.Exit(17)
}

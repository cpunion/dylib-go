//go:build cgo && (linux || darwin || windows)

package cunion_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func nativeFlags() []string {
	if runtime.GOARCH == "386" {
		if runtime.GOOS == "windows" {
			args := []string{"--target=i686-w64-windows-gnu"}
			if root := os.Getenv("DYLIB_NATIVE_SYSROOT"); root != "" {
				args = append(args, "--sysroot="+root, "--rtlib=libgcc")
			}
			return args
		}
		return []string{"-m32"}
	}
	return nil
}

func producer(t *testing.T, src, opt string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	native, err := filepath.Abs("testdata/values.c")
	if err != nil {
		t.Fatal(err)
	}
	objs := []string{}
	for i, path := range []string{src, native} {
		out := filepath.Join(dir, fmt.Sprintf("part%d.o", i))
		args := []string{"-std=c17", "-ffreestanding", "-fno-stack-protector", opt, "-c"}
		if runtime.GOOS != "windows" {
			args = append(args, "-fPIC")
		}
		args = append(args, nativeFlags()...)
		args = append(args, path, "-o", out)
		if output, err := exec.Command(clangTool(t), args...).CombinedOutput(); err != nil {
			t.Fatalf("C union producer: %v\n%s", err, output)
		}
		objs = append(objs, out)
	}
	archive := filepath.Join(dir, "values.a")
	ar := os.Getenv("AR")
	if ar == "" {
		ar = "ar"
	}
	if out, err := exec.Command(tool(t, ar), append([]string{"rcs", archive}, objs...)...).CombinedOutput(); err != nil {
		t.Fatalf("union archive: %v\n%s", err, out)
	}
	library := filepath.Join(dir, "values.so")
	flag := "-shared"
	if runtime.GOOS == "darwin" {
		flag = "-dynamiclib"
		library = filepath.Join(dir, "values.dylib")
	}
	if runtime.GOOS == "windows" {
		library = filepath.Join(dir, "values.dll")
	}
	args := append([]string{flag}, nativeFlags()...)
	args = append(args, objs...)
	args = append(args, "-o", library)
	if out, err := exec.Command(clangTool(t), args...).CombinedOutput(); err != nil {
		t.Fatalf("union library: %v\n%s", err, out)
	}
	// Preserve separately compiled native and facade objects for raw execution.
	return map[string]string{"archive": archive, "library": library, "object": strings.Join(objs, "\n")}
}
func TestNativeGeneratedCgoUnionFacade(t *testing.T) { testGenerated(t, false) }
func TestNativeGeneratedDirectLLGoUnionFacade(t *testing.T) {
	if !llgoHost {
		t.Skip("requires an llgo host")
	}
	testGenerated(t, true)
}
func testGenerated(t *testing.T, direct bool) {
	t.Helper()
	_, src, metadata := render(t)
	generate := metadata.CgoSource
	if direct {
		generate = metadata.LLGoSource
	}
	bindings, err := generate("main", "Bindings")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "bindings.go"), bindings)
	main, err := os.ReadFile("testdata/main.go")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "main.go"), main)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "go.mod"), []byte(fmt.Sprintf("module union-facade-test\n\ngo 1.23\n\nrequire github.com/cpunion/dylib-go v0.0.0\nreplace github.com/cpunion/dylib-go => %s\n", strconv.Quote(filepath.ToSlash(root)))))
	compiler := "go"
	if llgoHost {
		compiler = os.Getenv("DYLIB_LLGO")
		if compiler == "" {
			compiler = "llgo"
		}
	}
	binary := filepath.Join(dir, "union-runner")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	cmd := exec.Command(tool(t, compiler), "build", "-o", binary, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOARCH="+runtime.GOARCH)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated %s union caller: %v\n%s", compiler, err, out)
	}
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			for kind, path := range producer(t, src, opt) {
				t.Run(kind, func(t *testing.T) {
					args := []string{path}
					if out, err := exec.Command(binary, args...).CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "42" {
						t.Fatalf("%s union call: %v\n%s", compiler, err, out)
					}
				})
			}
		})
	}
}

//go:build cgo && (linux || darwin || windows)

package llvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	examplecall "github.com/cpunion/dylib-go/examples/call"
)

func TestNativeMergedModuleCallsAndInitializers(t *testing.T) {
	opts := mergeTools(t)
	clang := nativeClang(t)
	var inputs, directories []string
	for i, code := range []string{
		"static int seed;\n__attribute__((constructor)) static void init(void){seed=10;}\nstatic int same(int a){return a+seed;}\nint provided(int a,int b){return same(a+b);}\n",
		"extern int provided(int,int);\nstatic int same(int a){return a-10;}\nint add(int a,int b){return same(provided(a,b));}\n",
	} {
		dir := t.TempDir()
		directories = append(directories, dir)
		source := filepath.Join(dir, "module.c")
		if err := os.WriteFile(source, []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
		input := filepath.Join(dir, "module with spaces")
		flags := append(nativeCFlags(source, input), "-emit-llvm")
		if i == 1 {
			flags = append(flags, "-S")
		}
		if data, err := exec.Command(clang, flags...).CombinedOutput(); err != nil {
			t.Fatalf("module producer: %v\n%s", err, data)
		}
		inputs = append(inputs, input)
	}
	object, err := CompileModules(context.Background(), inputs, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer object.Close()
	if !reflect.DeepEqual(object.SourceDirectories, directories) {
		t.Fatal("original dependency directories lost:", object.SourceDirectories)
	}
	for _, input := range inputs {
		if err := os.Remove(input); err != nil {
			t.Fatal(err)
		}
	}
	session := dylib.New(dylib.Options{LibraryPaths: object.SourceDirectories})
	defer session.Close()
	if err := session.Load(object.Path); err != nil {
		t.Fatal(err)
	}
	if err := object.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.Link("add"); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		name string
		want int32
	}{{"add", 42}, {"provided", 52}} {
		fn, err := session.Resolve(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if err := fn.WithAddress(func(address uintptr) error {
			got, err := examplecall.Int32(address, 20, 22)
			if err != nil || got != entry.want {
				t.Fatalf("merged %s: %d, %v; want %d", entry.name, got, err, entry.want)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

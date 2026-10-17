package dylib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func coffCXXImportSources(t *testing.T, dir string) (string, string) {
	t.Helper()
	var producer, consumer strings.Builder
	for i, name := range []string{"add", "_add"} {
		fmt.Fprintf(&producer, "namespace api { __declspec(dllexport) int %s(int a,int b){return a+b+%d;} }\n", name, 8+i)
		fmt.Fprintf(&consumer, "namespace api { __declspec(dllimport) int %s(int,int); }\nextern \"C\" int invoke_cpp%d(int a,int b){return api::%s(a,b);}\n", name, i, name)
	}
	producer.WriteString("namespace api { __declspec(dllexport) int counter=42; }\n")
	consumer.WriteString("namespace api { __declspec(dllimport) extern int counter; }\nextern \"C\" int read_cpp(int a,int b){return a+b+api::counter-42;}\n")
	src, dst := filepath.Join(dir, "provider.cpp"), filepath.Join(dir, "consumer.cpp")
	if err := os.WriteFile(src, []byte(producer.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(consumer.String()), 0600); err != nil {
		t.Fatal(err)
	}
	return src, dst
}

func TestCOFFI386CXXImportNames(t *testing.T) {
	for _, target := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			_, src := coffCXXImportSources(t, dir)
			obj := compile(t, src, filepath.Join(dir, "consumer.o"), "--target="+target, "-ffreestanding", "-fno-exceptions", "-fno-rtti")
			data, err := readFile(obj)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse(obj, data)
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]bool{}
			for _, symbol := range f.obj.symbols {
				if symbol.section == 0 {
					names[symbol.name] = true
				}
			}
			wanted := []string{"__imp_?add@api@@YAHHH@Z", "__imp_?_add@api@@YAHHH@Z", "__imp_?counter@api@@3HA"}
			if strings.Contains(target, "gnu") {
				wanted = []string{"__imp__ZN3api3addEii", "__imp__ZN3api4_addEii", "__imp__ZN3api7counterE"}
			}
			for _, name := range wanted {
				if !names[name] {
					t.Fatalf("missing exact C++ import %q: %v", name, names)
				}
			}
		})
	}
}

func TestNativeCOFFI386CXXImports(t *testing.T) {
	var calls []coffImportCall
	for i, name := range []string{"add", "_add"} {
		calls = append(calls, coffImportCall{"wrapper_" + name, fmt.Sprintf("invoke_cpp%d", i), 50 + int32(i)}, coffImportCall{"native_" + name, "?" + name + "@api@@YAHHH@Z", 50 + int32(i)})
	}
	calls = append(calls, coffImportCall{"data", "read_cpp", 42})
	// Primitive free functions use cdecl here; C++ object/exception semantics
	// are intentionally absent, and the caller supplies the exact mangled name.
	nativeCOFFI386Imports(t, coffCXXImportSources, calls, "-fno-exceptions", "-fno-rtti")
}

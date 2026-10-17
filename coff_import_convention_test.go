package dylib

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type coffConventionCase struct{ name, convention, public string }

func coffConventionCases() []coffConventionCase {
	return []coffConventionCase{
		{"cdecl_add", "cdecl", "cdecl_add"},
		{"stdcall_add", "stdcall", "stdcall_add@8"},
		{"fastcall_add", "fastcall", "@fastcall_add@8"},
		{"vector_add", "vectorcall", "vector_add@@8"},
		{"_cdecl_add", "cdecl", "_cdecl_add"},
		{"_stdcall_add", "stdcall", "_stdcall_add@8"},
		{"_fastcall_add", "fastcall", "@_fastcall_add@8"},
		{"_vector_add", "vectorcall", "_vector_add@@8"},
	}
}
func coffConventionSources(t *testing.T, dir string) (string, string) {
	t.Helper()
	var producer, consumer strings.Builder
	for i, tc := range coffConventionCases() {
		fmt.Fprintf(&producer, "__declspec(dllexport) int __attribute__((%s)) %s(int a,int b){return a+b+%d;}\n", tc.convention, tc.name, i)
		fmt.Fprintf(&consumer, "__declspec(dllimport) int __attribute__((%s)) %s(int,int);\nint invoke%d(int a,int b){return %s(a,b);}\n", tc.convention, tc.name, i, tc.name)
	}
	src, dst := filepath.Join(dir, "provider.c"), filepath.Join(dir, "consumer.c")
	if err := os.WriteFile(src, []byte(producer.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(consumer.String()), 0600); err != nil {
		t.Fatal(err)
	}
	return src, dst
}
func TestCOFFI386ImportConventionNames(t *testing.T) {
	for _, target := range []string{"i686-pc-windows-msvc", "i686-w64-windows-gnu"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			_, src := coffConventionSources(t, dir)
			obj := compile(t, src, filepath.Join(dir, "consumer.o"), "--target="+target, "-ffreestanding")
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
			for _, tc := range coffConventionCases() {
				if name := "__imp_" + tc.public; !names[name] {
					t.Fatalf("missing exact %s import %q: %v", tc.convention, name, names)
				}
			}
		})
	}
}
func TestNativeCOFFI386ConventionImports(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "386" {
		t.Skip("native Windows i386 calling conventions")
	}
	needNative(t)
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			src, consumerSrc := coffConventionSources(t, dir)
			provider := compile(t, src, filepath.Join(dir, "provider.o"), "--target=i686-pc-windows-msvc", opt)
			consumer := compile(t, consumerSrc, filepath.Join(dir, "consumer.o"), "--target=i686-pc-windows-msvc", opt)
			dll, library := filepath.Join(dir, "provider.dll"), filepath.Join(dir, "provider.lib")
			command(t, "lld-link", "/dll", "/noentry", "/nodefaultlib", "/machine:X86", "/out:"+dll, "/implib:"+library, provider)
			archive := filepath.Join(dir, "consumer.a")
			command(t, "llvm-ar", "rcs", archive, consumer)
			for _, input := range []struct{ kind, path string }{{"object", consumer}, {"archive", archive}} {
				t.Run(input.kind, func(t *testing.T) {
					s := New(Options{})
					defer s.Close()
					load(t, s, library, input.path)
					for i, tc := range coffConventionCases() {
						t.Run(tc.name, func(t *testing.T) {
							// Repetition catches convention-specific stack cleanup errors.
							for n := int32(0); n < 100; n++ {
								call(t, s, fmt.Sprintf("invoke%d", i), 20+n, 22-n, 42+int32(i))
							}
						})
					}
					if len(s.libs) != 1 {
						t.Fatalf("DLL references were not shared: %d", len(s.libs))
					}
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					if _, err := callInt32(s, "invoke0", 20, 22); !errors.Is(err, ErrClosed) {
						t.Fatal("closed import provider", err)
					}
				})
			}
		})
	}
}

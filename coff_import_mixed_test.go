package dylib

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mixedImportAssembly(target string) string {
	prefix, pointer := "", ".quad "
	if strings.HasPrefix(target, "i686") {
		prefix, pointer = "_", ".long "
	}
	name := func(s string) string { return prefix + s }
	text := ".section .text$mix,\"xr\"\n.p2align 2\n.global " + name("mixed") + "\n" + name("mixed") + ":\n"
	branch := "jmp "
	if strings.HasPrefix(target, "aarch64") {
		branch = "b "
	}
	text += branch + name("add") + "\n.global " + name("mixed_dependency") + "\n" + name("mixed_dependency") + ":\n" + branch + name("dependency") + "\n"
	text += ".global " + name("mixed_read") + "\n" + name("mixed_read") + ":\n"
	switch {
	case strings.HasPrefix(target, "aarch64"):
		text += "adrp x9," + name("mixed_bias") + "\nldr w0,[x9,:lo12:" + name("mixed_bias") + "]\nadrp x9," + name("mixed_zero") + "\nldr w10,[x9,:lo12:" + name("mixed_zero") + "]\nadd w0,w0,w10\nret\n"
		text += "mixed_init:\nstp x29,x30,[sp,#-16]!\nmov w0,#20\nmov w1,#22\nbl " + name("add") + "\nadrp x9," + name("mixed_zero") + "\nstr w0,[x9,:lo12:" + name("mixed_zero") + "]\nldp x29,x30,[sp],#16\nret\n"
	case strings.HasPrefix(target, "i686"):
		text += "movl " + name("mixed_bias") + ",%eax\naddl " + name("mixed_zero") + ",%eax\nret\nmixed_init:\npushl $22\npushl $20\ncall " + name("add") + "\naddl $8,%esp\nmovl %eax," + name("mixed_zero") + "\nret\n"
	default:
		text += "movl " + name("mixed_bias") + "(%rip),%eax\naddl " + name("mixed_zero") + "(%rip),%eax\nret\nmixed_init:\nsubq $40,%rsp\nmovl $20,%ecx\nmovl $22,%edx\ncall " + name("add") + "\nmovl %eax," + name("mixed_zero") + "(%rip)\naddq $40,%rsp\nret\n"
	}
	text += ".data\n.p2align 2\n.global " + name("mixed_bias") + "\n" + name("mixed_bias") + ":\n.long 20\n.bss\n.p2align 2\n.global " + name("mixed_zero") + "\n" + name("mixed_zero") + ":\n.space 4\n"
	return text + ".section .CRT$XCU,\"dr\"\n" + pointer + "mixed_init\n.section .CRT$XTU,\"dr\"\n" + pointer + "mixed_init\n"
}

func mixedImportParts(t *testing.T, target, dll string) [][]byte {
	t.Helper()
	parts := longImportParts(t, target, dll)
	data, _ := coffAssembly(t, target, longImportAssembly(target, longImportFixture{"add", "add", 0, false})+mixedImportAssembly(target))
	return append(parts, data)
}

func TestCOFFMixedLongImports(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			parts := mixedImportParts(t, target, "provider.dll")
			raw, err := parse("mixed.obj", parts[2])
			if err != nil {
				t.Fatal(err)
			}
			f, err := parse("mixed.lib", longImportArchive(parts))
			if err != nil {
				t.Fatal(err)
			}
			o := f.members[2].obj
			if len(o.info.Unsupported) != 0 || len(o.info.Imports) != 1 || len(o.symbols) != len(raw.obj.symbols) || len(o.sections) != len(raw.obj.sections) {
				t.Fatalf("mixed import not preserved: %+v", o.info)
			}
			kept, lifecycle := 0, 0
			for i, sec := range o.sections {
				if sec == nil {
					continue
				}
				kept++
				if sec != raw.obj.sections[i] && (sec.name != raw.obj.sections[i].name || string(sec.data) != string(raw.obj.sections[i].data)) {
					t.Fatal("ordinary section changed")
				}
				if sec.lifecycle != 0 {
					lifecycle++
				}
				if sec.name == ".text" || strings.HasPrefix(sec.name, ".idata") {
					t.Fatal("native import template retained", sec.name)
				}
			}
			if kept != 5 || lifecycle != 2 {
				t.Fatalf("ordinary/lifecycle sections dropped: %d/%d", kept, lifecycle)
			}
			for i, s := range raw.obj.symbols {
				if s.name == "mixed" || s.name == "mixed_read" || s.name == "mixed_bias" || s.name == "mixed_zero" || s.name == "dependency" {
					if o.symbols[i] != s {
						t.Fatal("ordinary symbol indices changed", s.name)
					}
				}
			}
			count := 0
			for _, r := range raw.obj.relocs {
				if o.sections[r.section] != nil {
					if count >= len(o.relocs) || o.relocs[count] != r {
						t.Fatal("ordinary relocation changed")
					}
					count++
				}
			}
			if count == 0 || count != len(o.relocs) {
				t.Fatal("ordinary relocations dropped")
			}
			provider := &object{symbols: []symbol{{name: "dependency", global: true, section: -1, value: 0x10003000}}}
			defs, err := definitions([]*object{o, provider})
			if err != nil {
				t.Fatal(err)
			}
			im := &image{objects: []*object{o}, defs: defs, base: 0x10000000, mem: make([]byte, 8192), pointerSize: uint64(o.info.Bits / 8), got: map[uintptr]uintptr{}, gotStart: 4096, gotNext: 4096, stubStart: 8192, imports: make(map[*coffImport]uintptr)}
			for i, sec := range o.sections {
				if sec != nil {
					sec.offset = uint64(i) * 128
					copy(im.mem[sec.offset:], sec.data)
				}
			}
			for _, s := range o.symbols {
				if s.imported != nil {
					im.imports[s.imported.entry] = 0x12345678
				}
			}
			for _, r := range o.relocs {
				if err := im.relocate(o, r); err != nil {
					t.Fatal("ordinary relocation failed", err)
				}
			}
			if address, err := im.lookup("mixed"); err != nil || address == 0x12345678 {
				t.Fatal("ordinary code replaced by import", address, err)
			}
		})
	}
}

func TestCOFFMixedLongImportData(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			parts := longImportParts(t, target, "provider.dll")
			extra := "extra"
			if strings.HasPrefix(target, "i686") {
				extra = "_" + extra
			}
			data, _ := coffAssembly(t, target, longImportAssembly(target, longImportFixture{"value", "value", 0, true})+".data\n.global "+extra+"\n"+extra+":\n.long 42\n")
			f, err := parse("data.lib", longImportArchive(append(parts, data)))
			if err != nil {
				t.Fatal(err)
			}
			o := f.members[2].obj
			if len(o.info.Unsupported) != 0 || len(o.info.Imports) != 1 || o.info.Imports[0].Kind != "data" {
				t.Fatal("mixed data import not converted", o.info)
			}
			found := false
			for _, s := range o.symbols {
				found = found || s.name == "extra" && s.section > 0 && o.sections[s.section] != nil
			}
			if !found {
				t.Fatal("ordinary data definition dropped")
			}
		})
	}
}

func TestCOFFMixedLongImportKeepsUnsupported(t *testing.T) {
	const target = "x86_64-pc-windows-msvc"
	parts := longImportParts(t, target, "provider.dll")
	data, _ := coffAssembly(t, target, longImportAssembly(target, longImportFixture{"add", "add", 0, false})+mixedImportAssembly(target)+".section .tls$AAA,\"dw\"\n.long 0\n")
	f, err := parse("tls.lib", longImportArchive(append(parts, data)))
	if err != nil {
		t.Fatal(err)
	}
	o := f.members[2].obj
	if len(o.info.Imports) != 1 || len(o.info.Unsupported) != 1 || !strings.Contains(o.info.Unsupported[0], "TLS section") {
		t.Fatal("ordinary unsupported behavior hidden", o.info)
	}
}

func TestCOFFMixedLongImportValidation(t *testing.T) {
	parts := mixedImportParts(t, "x86_64-pc-windows-msvc", "provider.dll")
	for name, patch := range map[string]func(*object){
		"nonstandard_idata": func(o *object) { o.sections = append(o.sections, &section{name: ".idata$9", size: 8}) },
		"arbitrary_text": func(o *object) {
			for _, sec := range o.sections {
				if sec != nil && sec.name == ".text" {
					sec.data[0] = 0xc3
				}
			}
		},
		"comdat": func(o *object) { o.groups = []sectionGroup{{selection: 2}} },
		"local_iat_reference": func(o *object) {
			for i, s := range o.symbols {
				if s.name == ".idata$5" {
					o.relocs[len(o.relocs)-1].symbol = i
					return
				}
			}
		},
		"local_thunk_reference": func(o *object) {
			for i, s := range o.symbols {
				if s.name == ".text" {
					o.relocs[len(o.relocs)-1].symbol = i
					return
				}
			}
		},
		"unknown_section": func(o *object) { o.relocs[len(o.relocs)-1].section = len(o.sections) },
		"unknown_symbol":  func(o *object) { o.relocs[len(o.relocs)-1].symbol = len(o.symbols) },
	} {
		t.Run(name, func(t *testing.T) {
			var objects []*object
			for _, part := range parts {
				f, err := parse("raw", append([]byte(nil), part...))
				if err != nil {
					t.Fatal(err)
				}
				objects = append(objects, f.obj)
			}
			patch(objects[2])
			if _, ok := newCOFFImportGraph(objects).convert(objects[2]); ok {
				t.Fatal("unsupported mixed layout converted")
			}
		})
	}
}

func TestNativeCOFFMixedLongImports(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("mixed COFF import execution requires Windows")
	}
	needNative(t)
	target := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH] + "-pc-windows-msvc"
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "provider.c")
			if err := os.WriteFile(src, []byte("static int count; int add(int a,int b){++count;return a+b;} int events(int a,int b){return count;}"), 0600); err != nil {
				t.Fatal(err)
			}
			obj := compile(t, src, filepath.Join(dir, "provider.obj"), opt)
			def := filepath.Join(dir, "provider.def")
			if err := os.WriteFile(def, []byte("EXPORTS\nadd\nevents\n"), 0600); err != nil {
				t.Fatal(err)
			}
			dll := filepath.Join(dir, "provider.dll")
			machine := map[string]string{"amd64": "X64", "arm64": "ARM64", "386": "X86"}[runtime.GOARCH]
			command(t, "lld-link", "/dll", "/noentry", "/nodefaultlib", "/machine:"+machine, "/def:"+def, "/out:"+dll, obj)
			dep := filepath.Join(dir, "dependency.c")
			if err := os.WriteFile(dep, []byte("int dependency(int a,int b){return a+b+1;}"), 0600); err != nil {
				t.Fatal(err)
			}
			dependency := compile(t, dep, dep+".obj", opt)
			parts := mixedImportParts(t, target, "provider.dll")
			unused := longImportParts(t, target, "not-installed-mixed.dll")
			data, _ := coffAssembly(t, target, longImportAssembly(target, longImportFixture{"absent", "absent", 0, false})+".data\n.global unused_mixed\nunused_mixed:\n.long 1\n")
			unusedLib := filepath.Join(dir, "unused.lib")
			if err := os.WriteFile(unusedLib, longImportArchive(append(unused, data)), 0600); err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"archive", "retry"} {
				t.Run(mode, func(t *testing.T) {
					observer := New(Options{})
					defer observer.Close()
					load(t, observer, dll)
					call(t, observer, "events", 0, 0, 0)
					members := append([][]byte(nil), parts...)
					if mode == "archive" {
						data, err := os.ReadFile(dependency)
						if err != nil {
							t.Fatal(err)
						}
						members = append(members, data)
					}
					lib := filepath.Join(dir, mode+".lib")
					if err := os.WriteFile(lib, longImportArchive(members), 0600); err != nil {
						t.Fatal(err)
					}
					s := New(Options{})
					defer s.Close()
					load(t, s, lib, unusedLib)
					if mode == "retry" {
						if err := s.Link("mixed"); err == nil || !strings.Contains(err.Error(), "dependency") || s.image != nil {
							t.Fatal("ordinary dependency bypassed", err)
						}
						call(t, observer, "events", 0, 0, 0)
						load(t, s, dependency)
					}
					call(t, s, "mixed_read", 0, 0, 62)
					call(t, s, "mixed_dependency", 20, 22, 43)
					for i := int32(0); i < 100; i++ {
						call(t, s, "mixed", 20+i, 22, 42+i)
					}
					call(t, observer, "events", 0, 0, 101)
					if len(s.image.objects) != 2 || len(s.libs) != 1 {
						t.Fatal("metadata helpers extracted or DLL references duplicated")
					}
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					call(t, observer, "events", 0, 0, 102)
					if _, err := s.Lookup("mixed"); !errors.Is(err, ErrClosed) {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

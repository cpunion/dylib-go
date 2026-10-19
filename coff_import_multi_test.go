package dylib

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func multiImportAssembly(target string, entries []longImportFixture) string {
	var text strings.Builder
	for i, entry := range entries {
		hint := fmt.Sprintf("hint%d", i)
		text.WriteString(strings.NewReplacer(".rva hint\n", ".rva "+hint+"\n", "hint:\n", hint+":\n").Replace(longImportAssembly(target, entry)))
	}
	return text.String()
}

func TestNativeCOFFMultiLongImports(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DLL imports require Windows")
	}
	needNative(t)
	dll, _, _ := nativeImportFixture(t)
	dir := filepath.Dir(dll)
	target := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH] + "-w64-windows-gnu"
	entries := []longImportFixture{{"add", "add", 0, false}, {"renamed", "add", 0, false}, {"numbered", "", 7, false}, {"value", "value", 0, true}}
	parts := multiImportParts(t, target, "provider.dll", "", entries...)
	lib := filepath.Join(dir, "multi.lib")
	if err := os.WriteFile(lib, longImportArchive(parts), 0600); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "multi-consumer.c")
	if err := os.WriteFile(src, []byte("__declspec(dllimport) int add(int,int),renamed(int,int),numbered(int,int);\n__declspec(dllimport) extern int value;\nint multi_imported(int a,int b){return add(a,b)+renamed(a,b)+numbered(a,b)+value-126;}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, opt := range []string{"-O0", "-O2"} {
		t.Run(opt, func(t *testing.T) {
			obj := compile(t, src, src+opt+".obj", "--target="+target, opt)
			s := New(Options{})
			defer s.Close()
			load(t, s, lib, obj)
			if err := s.Link("multi_imported", "add", "renamed", "numbered"); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"multi_imported", "add", "renamed", "numbered"} {
				call(t, s, name, 20, 22, 42)
			}
			if len(s.libs) != 1 || len(s.image.objects) != 2 {
				t.Fatal("import helpers extracted or DLL handles duplicated")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCOFFLongImportDuplicateThunkRelocations(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			var objects []*object
			for _, data := range longImportParts(t, target, "provider.dll", longImportFixture{"add", "add", 0, false}) {
				f, err := parse("raw.obj", data)
				if err != nil {
					t.Fatal(err)
				}
				objects = append(objects, f.obj)
			}
			o := objects[2]
			var first relocation
			found := false
			for i, r := range o.relocs {
				if o.sections[r.section].name != ".text" {
					continue
				}
				if !found {
					first, found = r, true
				} else {
					o.relocs[i] = first
				}
			}
			if !found {
				t.Fatal("fixture has no thunk relocations")
			}
			if !strings.HasPrefix(target, "aarch64") {
				o.relocs = append(o.relocs, first)
			}
			if _, ok := newCOFFImportGraph(objects).convert(o); ok {
				t.Fatal("duplicate thunk site accepted in place of a required relocation")
			}
		})
	}
}

func multiImportParts(t *testing.T, target, dll, extra string, entries ...longImportFixture) [][]byte {
	t.Helper()
	parts := longImportParts(t, target, dll)
	data, _ := coffAssembly(t, target, multiImportAssembly(target, entries)+extra)
	return append(parts, data)
}

func TestCOFFMultiLongImports(t *testing.T) {
	entries := []longImportFixture{{"add", "add", 0, false}, {"renamed", "different_export", 0, false}, {"numbered", "", 7, false}, {"value", "value", 0, true}}
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			for _, mode := range []string{"pure", "mixed", "comdat"} {
				t.Run(mode, func(t *testing.T) {
					extra := ""
					if mode == "mixed" {
						extra = mixedImportAssembly(target)
					} else if mode == "comdat" {
						extra = mixedCOMDATImportAssembly(target)
					}
					parts := multiImportParts(t, target, "provider.dll", extra, entries...)
					raw, err := parse("raw.obj", parts[2])
					if err != nil {
						t.Fatal(err)
					}
					f, err := parse("multi.lib", longImportArchive(parts))
					if err != nil {
						t.Fatal(err)
					}
					o := f.members[2].obj
					if len(o.info.Imports) != len(entries) || len(o.info.Unsupported) != 0 {
						t.Fatalf("multiple imports not normalized: %+v", o.info)
					}
					for i, want := range entries {
						got := o.info.Imports[i]
						kind := "code"
						if want.data {
							kind = "data"
						}
						if got != (ImportInfo{DLL: "provider.dll", Symbol: want.public, Name: want.export, Ordinal: want.ordinal, Kind: kind}) {
							t.Fatalf("entry %d: %+v", i, got)
						}
					}
					if mode == "pure" {
						if len(o.sections) != 0 || len(o.relocs) != 0 {
							t.Fatal("import-only template retained native storage")
						}
					} else {
						if len(o.symbols) != len(raw.obj.symbols) || len(o.sections) != len(raw.obj.sections) || !reflect.DeepEqual(o.groups, raw.obj.groups) {
							t.Fatal("ordinary symbol/section/group indices changed")
						}
						for i, symbol := range raw.obj.symbols {
							if symbol.section > 0 && o.sections[symbol.section] != nil && o.symbols[i] != symbol {
								t.Fatal("ordinary symbol changed", symbol.name)
							}
						}
						var kept []relocation
						for _, r := range raw.obj.relocs {
							if o.sections[r.section] != nil {
								kept = append(kept, r)
							}
						}
						if len(kept) == 0 || !reflect.DeepEqual(o.relocs, kept) {
							t.Fatal("ordinary relocations changed")
						}
					}
					seen := make(map[*coffImport]bool)
					for _, symbol := range o.symbols {
						if symbol.imported != nil {
							seen[symbol.imported.entry] = true
						}
					}
					if len(seen) != len(entries) {
						t.Fatal("distinct IAT entries share one import identity")
					}
				})
			}
		})
	}
}

func TestCOFFMultiLongImportLarge(t *testing.T) {
	entries := make([]longImportFixture, 513) // Cross a 4 KiB IAT boundary on 64-bit targets.
	for i := range entries {
		entries[i] = longImportFixture{public: fmt.Sprintf("item%d", i), export: "add"}
	}
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			parts := multiImportParts(t, target, "provider.dll", "", entries...)
			f, err := parse("large.lib", longImportArchive(parts))
			if err != nil {
				t.Fatal(err)
			}
			imports := f.members[2].info.Imports
			if len(imports) != len(entries) || imports[len(imports)-1].Symbol != "item512" || imports[len(imports)-1].Name != "add" || len(f.members[2].info.Unsupported) != 0 {
				t.Fatal("large IAT member not preserved")
			}
		})
	}
}

func TestCOFFMultiLongImportForms(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			for _, mode := range []string{"data_first", "data_only", "ordinal_only", "one_descriptor", "no_ilt", "constant"} {
				t.Run(mode, func(t *testing.T) {
					entries := []longImportFixture{{"value", "value", 0, true}, {"add", "add", 0, false}, {"other", "other", 0, true}, {"numbered", "", 7, false}}
					if mode == "data_only" || mode == "constant" {
						entries = []longImportFixture{{"value", "value", 0, true}, {"constant", "constant", 0, true}}
					} else if mode == "ordinal_only" {
						entries = []longImportFixture{{"one", "", 7, false}, {"two", "", 8, true}}
					}
					assembly := multiImportAssembly(target, entries)
					if mode == "one_descriptor" {
						force := ".section .idata$7,\"dr\"\n.rva dll_head\n"
						assembly = strings.ReplaceAll(assembly, force, "") + force
					} else if mode == "no_ilt" {
						chunks := strings.Split(assembly, ".section .idata$4,\"dr\"\n")
						assembly = chunks[0]
						for _, chunk := range chunks[1:] {
							_, rest, ok := strings.Cut(chunk, ".section .idata$5,\"dr\"\n")
							if !ok {
								t.Fatal("missing fixture IAT")
							}
							assembly += ".section .idata$5,\"dr\"\n" + rest
						}
					} else if mode == "constant" {
						name := "constant"
						if strings.HasPrefix(target, "i686") {
							name = "_" + name
						}
						assembly += ".global " + name + "\n.set " + name + ",__imp_" + name + "\n"
					}
					data, _ := coffAssembly(t, target, assembly)
					parts := append(longImportParts(t, target, "provider.dll"), data)
					f, err := parse("forms.lib", longImportArchive(parts))
					if err != nil || len(f.members[2].info.Imports) != len(entries) || len(f.members[2].info.Unsupported) != 0 {
						t.Fatalf("%s: %v, %+v", mode, err, f)
					}
					for i, entry := range entries {
						kind := "code"
						if entry.data {
							kind = "data"
						}
						if mode == "constant" && entry.public == "constant" {
							kind = "const"
						}
						got := f.members[2].info.Imports[i]
						if got != (ImportInfo{DLL: "provider.dll", Symbol: entry.public, Name: entry.export, Ordinal: entry.ordinal, Kind: kind}) {
							t.Fatalf("entry %d: %+v", i, got)
						}
					}
				})
			}
		})
	}
}

func TestCOFFMultiLongImportValidation(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			parts := multiImportParts(t, target, "provider.dll", mixedImportAssembly(target), longImportFixture{"add", "add", 0, false}, longImportFixture{"other", "other", 0, false})
			for _, mode := range []string{"iat_size", "force_size", "missing_iat", "missing_ilt", "missing_force", "missing_public", "duplicate_iat", "duplicate_text", "ordinal_relocation", "extra_global", "unaligned_symbol", "duplicate_public", "wrong_thunk_slot", "wrong_ilt_hint", "arbitrary_thunk", "local_iat", "local_thunk", "different_dll"} {
				t.Run(mode, func(t *testing.T) {
					var objects []*object
					for _, part := range parts {
						f, err := parse("raw.obj", append([]byte(nil), part...))
						if err != nil {
							t.Fatal(err)
						}
						objects = append(objects, f.obj)
					}
					o := objects[2]
					indexes := make(map[string]int)
					for i, s := range o.sections {
						if s != nil {
							indexes[s.name] = i
						}
					}
					iat, ilt, text, force := indexes[".idata$5"], indexes[".idata$4"], indexes[".text"], indexes[".idata$7"]
					width := uint64(o.info.Bits / 8)
					patched := false
					for i, r := range o.relocs {
						remove := mode == "missing_iat" && r.section == iat && r.offset == width || mode == "missing_ilt" && r.section == ilt && r.offset == width || mode == "missing_force" && r.section == force && r.offset == 4
						if remove {
							o.relocs = append(o.relocs[:i], o.relocs[i+1:]...)
							patched = true
							break
						}
						if mode == "duplicate_iat" && r.section == iat || mode == "duplicate_text" && r.section == text {
							o.relocs = append(o.relocs, r)
							patched = true
							break
						}
						if mode == "wrong_thunk_slot" && r.section == text {
							for j, s := range o.symbols {
								if s.global && s.section == iat && s.value == width {
									o.relocs[i].symbol = j
									patched = true
									break
								}
							}
							break
						}
						if mode == "wrong_ilt_hint" && r.section == ilt && r.offset == width {
							s := o.symbols[r.symbol]
							s.value++
							o.relocs[i].symbol = len(o.symbols)
							o.symbols = append(o.symbols, s)
							patched = true
							break
						}
						if mode == "local_iat" || mode == "local_thunk" {
							if o.sections[r.section].name != ".CRT$XCU" {
								continue
							}
							section := iat
							if mode == "local_thunk" {
								section = text
							}
							o.relocs[i].symbol = len(o.symbols)
							o.symbols = append(o.symbols, symbol{section: section})
							patched = true
							break
						}
					}
					switch mode {
					case "iat_size":
						o.sections[iat].size--
						patched = true
					case "force_size":
						o.sections[force].size = 6
						patched = true
					case "ordinal_relocation":
						for _, index := range []int{iat, ilt} {
							o.sections[index].data[0] = 7
							o.sections[index].data[width-1] |= 0x80
							patched = true
						}
					case "extra_global":
						o.symbols = append(o.symbols, symbol{name: "extra", section: indexes[".idata$6"], global: true})
						patched = true
					case "unaligned_symbol", "duplicate_public", "missing_public":
						for i, s := range o.symbols {
							if s.global && s.section == iat && s.value == width {
								o.symbols[i].value = 1
								if mode == "duplicate_public" {
									o.symbols[i].value = 0
								} else if mode == "missing_public" {
									o.symbols[i].global = false
									o.symbols[i].value = s.value
								}
								patched = true
								break
							}
						}
					case "arbitrary_thunk":
						o.sections[text].data[0] = 0xc3
						patched = true
					case "different_dll":
						head, _ := coffAssembly(t, target, ".section .idata$2,\"dr\"\n.global other_head\nother_head:\n.long 0,0,0\n.rva other_name\n.long 0\n")
						tail, _ := coffAssembly(t, target, ".section .idata$7,\"dr\"\n.global other_name\nother_name:\n.asciz \"different.dll\"\n")
						for _, data := range [][]byte{head, tail} {
							f, err := parse("other.obj", data)
							if err != nil {
								t.Fatal(err)
							}
							objects = append(objects, f.obj)
						}
						for i, r := range o.relocs {
							if r.section == force && r.offset == 4 {
								o.relocs[i].symbol = len(o.symbols)
								o.symbols = append(o.symbols, symbol{name: "other_head", global: true})
								patched = true
							}
						}
					}
					if !patched {
						t.Fatal("fixture mutation not applied")
					}
					if _, ok := newCOFFImportGraph(objects).convert(o); ok {
						t.Fatal("invalid multi-import member accepted")
					}
					if len(o.info.Imports) != 0 {
						t.Fatal("failed conversion published partial imports")
					}
					for _, s := range o.symbols {
						if s.imported != nil {
							t.Fatal("failed conversion mutated original symbols")
						}
					}
				})
			}
		})
	}
}

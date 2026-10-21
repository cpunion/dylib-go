package dylib

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func machoCustomWidths(arch string) []int {
	if arch == "amd64" {
		return []int{5, 6, 7, 16}
	}
	return []int{4, 8, 12, 16}
}

func TestMachOCustomStubRelocations(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, width := range machoCustomWidths(arch) {
			for _, far := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/width%d/far%t", arch, width, far), func(t *testing.T) {
					const base, offset = 0x100000, 128
					target := uintptr(base + 1024)
					if far {
						farTarget := uint64(0x9000000000)
						target = uintptr(farTarget)
						if uint64(target) != farTarget {
							// Still exhaust rel32 and BRANCH26 on a 32-bit host.
							target = 0xf0000000
						}
					}
					o := &object{info: Info{Format: "Mach-O", Arch: arch}, sections: []*section{nil, {size: uint64(width), offset: offset}}, symbols: []symbol{{section: -1, value: uint64(target)}}}
					im := &image{base: base, mem: make([]byte, 4096), pointerSize: 8, got: map[uintptr]uintptr{}, stubs: map[uintptr]uintptr{}, gotStart: 2048, gotNext: 2048, stubStart: 3072, stubNext: 3072}
					im.mem[offset+width] = 0xcc
					r := relocation{section: 1, width: width, typ: machoStubFixup, pair: -1}
					if err := im.relocate(o, r); err != nil {
						t.Fatal(err)
					}
					code := im.mem[offset : offset+width]
					short := arch == "amd64" && width == 5 || arch == "arm64" && width < 12
					used := 6
					if arch == "arm64" {
						used = 12
					}
					if short {
						var destination uintptr
						if arch == "amd64" {
							used = 5
							if code[0] != 0xe9 {
								t.Fatal("short x86 stub is not a tail jump")
							}
							destination = uintptr(int64(base+offset+5) + int64(int32(le.Uint32(code[1:]))))
						} else {
							used = 4
							instruction := le.Uint32(code)
							if instruction&0xfc000000 != 0x14000000 {
								t.Fatal("short ARM stub changes the link register")
							}
							destination = uintptr(int64(base+offset) + signExtend(instruction&0x3ffffff, 26)*4)
						}
						want := target
						if far {
							want = im.stubs[target]
							if want == 0 {
								t.Fatal("missing absolute branch island")
							}
							literal := 6
							if arch == "arm64" {
								literal = 8
							}
							if got := le.Uint64(im.mem[want-base+uintptr(literal):]); got != uint64(target) {
								t.Fatal("island lost the full target:", got)
							}
						}
						if destination != want {
							t.Fatalf("branch %#x, want %#x", destination, want)
						}
					} else {
						slot := im.got[target]
						if slot == 0 || le.Uint64(im.mem[slot-base:]) != uint64(target) {
							t.Fatal("missing full-address GOT target")
						}
					}
					for i := used; i < width; {
						if arch == "amd64" {
							if code[i] != 0x90 {
								t.Fatal("noncanonical x86 padding")
							}
							i++
						} else {
							if le.Uint32(code[i:]) != 0xd503201f {
								t.Fatal("noncanonical ARM padding")
							}
							i += 4
						}
					}
					if im.mem[offset+width] != 0xcc {
						t.Fatal("stub overwrote its neighbor")
					}
				})
			}
		}
	}
}

func TestMachOCustomStubObjects(t *testing.T) {
	for _, target := range []struct{ triple, arch string }{{"x86_64-apple-macos11", "amd64"}, {"arm64-apple-macos11", "arm64"}} {
		for _, width := range machoCustomWidths(target.arch) {
			t.Run(fmt.Sprintf("%s/width%d", target.arch, width), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "stubs.o")
				command(t, compiler(), "--target="+target.triple, "-DSTUB_WIDTH="+fmt.Sprint(width), "-c", "testdata/macho_custom_stubs.S", "-o", path)
				b, err := readFile(path)
				if err != nil {
					t.Fatal(err)
				}
				o := parseMachoAliases(t, b)
				if len(o.relocs) != 2 || o.relocs[0].offset != 0 || o.relocs[1].offset != uint64(width) {
					t.Fatal("incorrect indirect stub stride:", o.relocs)
				}
				if target.arch == "arm64" {
					header := machoPointerHeader(t, b, "__stubs")
					le.PutUint32(b[header+52:], 0)
					if _, err := parse("unaligned-stubs", b); err == nil || !strings.Contains(err.Error(), "instruction alignment") {
						t.Fatal("accepted unaligned instruction storage:", err)
					}
				}
			})
		}
	}
}

func TestNativeMachOCustomStubs(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O execution requires macOS")
	}
	needNative(t)
	for _, width := range machoCustomWidths(runtime.GOARCH) {
		t.Run(fmt.Sprintf("width%d", width), func(t *testing.T) {
			for _, opt := range []string{"-O0", "-O2"} {
				t.Run(opt, func(t *testing.T) {
					for _, records := range []bool{false, true} {
						t.Run(fmt.Sprintf("records%t", records), func(t *testing.T) {
							dir := t.TempDir()
							flags := []string{opt}
							if records {
								flags = append(flags, "-DRECORDS")
							}
							stubs := compile(t, "testdata/macho_custom_stubs.S", filepath.Join(dir, "stubs.o"), "-DSTUB_WIDTH="+fmt.Sprint(width))
							provider := compile(t, "testdata/macho_custom_stubs.c", filepath.Join(dir, "provider.o"), append(flags, "-DPROVIDER_ONLY")...)
							consumer := compile(t, "testdata/macho_custom_stubs.c", filepath.Join(dir, "consumer.o"), append(flags, "-DCONSUMER_ONLY")...)
							unused := compile(t, "testdata/missing.c", filepath.Join(dir, "unused.o"))
							archive := filepath.Join(dir, "stubs.a")
							command(t, "ar", "rcs", archive, unused, stubs, provider)
							for _, kind := range []string{"objects", "archive", "provider"} {
								t.Run(kind, func(t *testing.T) {
									s := New(Options{})
									defer s.Close()
									switch kind {
									case "objects":
										load(t, s, consumer, stubs, provider)
									case "archive":
										load(t, s, consumer, archive)
									case "provider":
										host := New(Options{})
										defer func() {
											s.Close()
											host.Close()
										}()
										load(t, host, provider)
										load(t, s, consumer, stubs)
										if err := s.Link("consume_custom"); err == nil || !strings.Contains(err.Error(), "unresolved symbol") {
											t.Fatal("missing stub target:", err)
										}
										if s.image != nil {
											t.Fatal("missing target published an image")
										}
										for _, name := range []string{"eval", "other"} {
											symbol, err := host.Resolve(name)
											if err != nil {
												t.Fatal(err)
											}
											if err := s.DefineSymbol(name, symbol); err != nil {
												t.Fatal(err)
											}
										}
									}
									for i := int32(0); i < 100; i++ {
										want := 40 + 2*i
										if records {
											want = 283 + 2*i
										}
										call(t, s, "consume_custom", 20+i, 22, want)
									}
									if err := s.Close(); err != nil {
										t.Fatal(err)
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

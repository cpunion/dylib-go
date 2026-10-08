package dylib

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func largeCOFF(t *testing.T, target string, count int) ([]byte, *object) {
	t.Helper()
	var asm strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&asm, ".section .pad%d,\"dr\"\n", i)
	}
	code, word := "movl $42, %eax\nret\n", ".quad "
	if strings.HasPrefix(target, "aarch64") {
		code = "mov w0, #42\nret\n"
	}
	if strings.HasPrefix(target, "i686") {
		word = ".long "
	}
	asm.WriteString(".section .text$large,\"xr\",discard,large\n.globl large\nlarge:\n" + code)
	asm.WriteString(".section .rdata$association,\"dr\",associative,large\n.long 42\n")
	asm.WriteString(".weak optional\n.set optional,large\n.data\n" + word + "optional\n")
	return coffAssembly(t, target, asm.String())
}

func TestForeignCOFFLargeSectionNumbers(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		for _, count := range []int{33000, 65540} {
			t.Run(fmt.Sprintf("%s/%d", target, count), func(t *testing.T) {
				data, o := largeCOFF(t, target, count)
				big := le.Uint32(data) == 0xffff0000
				if big != (count > 65279) {
					t.Fatalf("unexpected compiler format: bigobj=%v", big)
				}
				defs, err := definitions([]*object{o})
				if err != nil || defs["large"].sym().section <= count || len(o.info.Unsupported) != 0 {
					t.Fatalf("high-section definition: %v %+v", err, o.info.Unsupported)
				}
				if len(o.groups) != 2 || o.groups[1].selection != 5 || o.groups[1].parent != defs["large"].sym().section {
					t.Fatalf("associative parent: %+v", o.groups)
				}
				selected, err := coalesceObjects([]*object{o, o})
				if err != nil {
					t.Fatal(err)
				}
				if selected[1].sections[o.groups[0].sections[0]] != nil || selected[1].sections[o.groups[1].sections[0]] != nil {
					t.Fatal("discarded high-section COMDAT retained")
				}
				aliases, err := weakAliases(selected)
				if err != nil {
					t.Fatal(err)
				}
				defs, err = definitions(selected)
				if err != nil {
					t.Fatal(err)
				}
				im := &image{objects: selected, defs: defs, aliases: aliases}
				d, err := im.resolveDefinition(aliases["optional"].o, aliases["optional"].index)
				if err != nil || d.o != selected[0] || d.sym().section <= count {
					t.Fatalf("high-section weak fallback: %+v %v", d, err)
				}
				archive := append([]byte("!<arch>\n"), archiveMemberBytes("large.obj/", data)...)
				f, err := parse("large.lib", archive)
				if err != nil || len(f.members) != 1 || f.members[0].info.Format != "COFF" {
					t.Fatalf("large archive: %+v %v", f, err)
				}
			})
		}
	}
}

// Promote a small real object to bigobj for compact malformed-input tests.
// Native high-section tests use Clang's genuine bigobj output instead.
func promoteBigCOFF(t *testing.T, data []byte) []byte {
	t.Helper()
	f, err := readCOFF(data)
	if err != nil || f.big || f.kind != "object" {
		t.Fatalf("promotion input: %v", err)
	}
	oldoff := int(le.Uint32(data[8:]))
	oldcount := int(le.Uint32(data[12:]))
	b := make([]byte, 56)
	le.PutUint16(b[2:], 0xffff)
	le.PutUint16(b[4:], 2)
	le.PutUint16(b[6:], f.machine)
	le.PutUint32(b[8:], f.timestamp)
	copy(b[12:], coffBigMagic)
	le.PutUint32(b[44:], uint32(len(f.sections)))
	le.PutUint32(b[48:], uint32(oldoff+36))
	le.PutUint32(b[52:], uint32(oldcount))
	body := append([]byte(nil), data[20:oldoff]...)
	for i := range f.sections {
		for _, field := range []int{20, 24, 28} {
			p := i*40 + field
			if v := le.Uint32(body[p:]); v != 0 {
				le.PutUint32(body[p:], v+36)
			}
		}
	}
	b = append(b, body...)
	for i := 0; i < oldcount; {
		s := f.symbol(i)
		entry := make([]byte, 20)
		copy(entry, f.record(i)[:12])
		le.PutUint32(entry[12:], uint32(s.section))
		le.PutUint16(entry[16:], s.typ)
		entry[18], entry[19] = s.class, s.aux
		b = append(b, entry...)
		for j := 1; j <= int(s.aux); j++ {
			entry = make([]byte, 20)
			copy(entry, f.record(i+j))
			b = append(b, entry...)
		}
		i += 1 + int(s.aux)
	}
	return append(b, data[oldoff+oldcount*18:]...)
}

func TestBigCOFFMalformed(t *testing.T) {
	data, _ := coffAssembly(t, "x86_64-pc-windows-msvc", ".text\n.globl value\nvalue:\nret\n")
	big := promoteBigCOFF(t, data)
	if _, err := parse("valid", big); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		patch func([]byte) []byte
	}{
		{"short header", func(b []byte) []byte { return b[:55] }},
		{"version", func(b []byte) []byte { le.PutUint16(b[4:], 1); return b }},
		{"uuid", func(b []byte) []byte { b[12] ^= 1; return b }},
		{"sections", func(b []byte) []byte { le.PutUint32(b[44:], 0xffffffff); return b }},
		{"symbol offset", func(b []byte) []byte { le.PutUint32(b[48:], 0xffffffff); return b }},
		{"symbol count", func(b []byte) []byte { le.PutUint32(b[52:], 0xffffffff); return b }},
		{"missing symbol offset", func(b []byte) []byte { le.PutUint32(b[48:], 0); return b }},
		{"string size", func(b []byte) []byte {
			p := le.Uint32(b[48:]) + 20*le.Uint32(b[52:])
			le.PutUint32(b[p:], 0xffffffff)
			return b
		}},
		{"aux count", func(b []byte) []byte { p := le.Uint32(b[48:]); b[p+19] = 255; return b }},
		{"section index", func(b []byte) []byte { p := le.Uint32(b[48:]); le.PutUint32(b[p+12:], 0x7fffffff); return b }},
		{"raw data", func(b []byte) []byte { le.PutUint32(b[56+20:], 0xffffffff); return b }},
		{"missing raw data", func(b []byte) []byte { le.PutUint32(b[56+20:], 0); return b }},
		{"relocation count", func(b []byte) []byte { le.PutUint16(b[56+32:], 1); le.PutUint32(b[56+24:], uint32(len(b))); return b }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parse(tc.name, tc.patch(append([]byte(nil), big...))); err == nil {
				t.Fatal("accepted malformed bigobj")
			}
		})
	}
}

func overflowCOFF(t *testing.T, target string) ([]byte, *object) {
	t.Helper()
	code, word := "movl $42, %eax\nret\n", ".quad "
	if strings.HasPrefix(target, "aarch64") {
		code = "mov w0, #42\nret\n"
	}
	if strings.HasPrefix(target, "i686") {
		word = ".long "
	}
	return coffAssembly(t, target, ".text\n.globl value\nvalue:\n"+code+".data\n.rept 65536\n"+word+"value\n.endr\n")
}

func TestCOFFRelocationOverflow(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			data, o := overflowCOFF(t, target)
			if len(o.relocs) != 65536 {
				t.Fatalf("relocation count: %d", len(o.relocs))
			}
			if _, err := parse("big-overflow", promoteBigCOFF(t, data)); err != nil {
				t.Fatal(err)
			}
			position := 0
			for i := 0; i < int(le.Uint16(data[2:])); i++ {
				p := 20 + i*40
				if le.Uint32(data[p+36:])&0x1000000 != 0 {
					position = p
					break
				}
			}
			if position == 0 {
				t.Fatal("compiler did not generate extended relocations")
			}
			for _, patch := range []func([]byte){
				func(b []byte) { le.PutUint16(b[position+32:], 1) },
				func(b []byte) { le.PutUint32(b[position+24:], 0) },
				func(b []byte) { p := le.Uint32(b[position+24:]); le.PutUint32(b[p:], 0xffffffff) },
				func(b []byte) { p := le.Uint32(b[position+24:]); le.PutUint32(b[p:], 1) },
				func(b []byte) { p := le.Uint32(b[position+24:]); le.PutUint32(b[p+4:], 1) },
			} {
				b := append([]byte(nil), data...)
				patch(b)
				if _, err := parse("bad-overflow", b); err == nil {
					t.Fatal("accepted malformed extended relocations")
				}
			}
		})
	}
}

func TestNativeCOFFRelocationOverflow(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("COFF execution needs Windows")
	}
	needNative(t)
	target := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH] + "-pc-windows-msvc"
	data, _ := overflowCOFF(t, target)
	path := filepath.Join(t.TempDir(), "overflow.obj")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s := New(Options{})
	defer s.Close()
	load(t, s, path)
	call(t, s, "value", 20, 22, 42)
	address, err := s.Lookup("value")
	if err != nil {
		t.Fatal(err)
	}
	// Check the final relocated table entry as well as calling the code.
	for _, sec := range s.image.objects[0].sections {
		if sec != nil && sec.name == ".data" {
			b := s.image.mem[sec.offset+sec.size-s.image.pointerSize:]
			got := uint64(le.Uint32(b))
			if s.image.pointerSize == 8 {
				got = le.Uint64(b)
			}
			if got != uint64(address) {
				t.Fatalf("last relocation: %#x, want %#x", got, address)
			}
			return
		}
	}
	t.Fatal("missing relocation table section")
}

func TestNativeBigCOFFObjectsAndArchives(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("COFF execution needs Windows")
	}
	needNative(t)
	target := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH] + "-pc-windows-msvc"
	data, _ := largeCOFF(t, target, 65540)
	for _, archive := range []bool{false, true} {
		b := data
		if archive {
			b = append([]byte("!<arch>\n"), archiveMemberBytes("large.obj/", data)...)
		}
		path := filepath.Join(t.TempDir(), "large.input")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
		s := New(Options{})
		load(t, s, path)
		call(t, s, "optional", 20, 22, 42)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

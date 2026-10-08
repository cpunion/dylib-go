package dylib

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestForeignCOFFExactMatchAndNewest(t *testing.T) {
	for _, target := range []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc", "i686-pc-windows-msvc"} {
		t.Run(target, func(t *testing.T) {
			data, a := coffAssembly(t, target, ".section .data$shared,\"dw\",same_contents,shared\n.globl shared\nshared:\n.long 42\n")
			_, b := coffAssembly(t, target, ".section .data$shared,\"dw\",same_contents,shared\n.globl shared\nshared:\n.long 42\n")
			out, err := coalesceObjects([]*object{a, b})
			if err != nil {
				t.Fatal(err)
			}
			defs, err := definitions(out)
			if err != nil || defs["shared"].o != out[0] {
				t.Fatalf("EXACT_MATCH selection: %v", err)
			}
			sec := b.groups[0].sections[0]
			b.sections[sec].data = append([]byte(nil), b.sections[sec].data...)
			b.sections[sec].data[0] = 43
			if _, err := coalesceObjects([]*object{a, b}); err == nil {
				t.Fatal("accepted unequal EXACT_MATCH definitions")
			}
			// NEWEST uses the COFF time stamp; equal stamps retain input order.
			for _, o := range []*object{a, b} {
				o.groups[0].selection = 7
			}
			a.timestamp, b.timestamp = 10, 20
			for _, inputs := range [][]*object{{a, b}, {b, a}} {
				out, err := coalesceObjects(inputs)
				if err != nil {
					t.Fatal(err)
				}
				defs, err := definitions(out)
				if err != nil || defs["shared"].o.timestamp != 20 {
					t.Fatalf("NEWEST selection: %v", err)
				}
			}
			b.timestamp = 10
			out, err = coalesceObjects([]*object{a, b})
			if err != nil {
				t.Fatal(err)
			}
			defs, err = definitions(out)
			if err != nil || defs["shared"].o != out[0] {
				t.Fatalf("equal-stamp selection: %v", err)
			}
			// Verify that the parser retains the timestamp and accepts rule 7.
			le.PutUint32(data[4:], 123)
			for i, v := range a.symbols {
				if v.section == a.groups[0].sections[0] && !v.global {
					aux := int(le.Uint32(data[8:])) + (i+1)*18
					data[aux+14] = 7
					break
				}
			}
			f, err := parse("newest", data)
			if err != nil || f.obj.timestamp != 123 || f.obj.groups[0].selection != 7 || len(f.info.Unsupported) != 0 {
				t.Fatalf("NEWEST metadata: %v %+v", err, f)
			}
		})
	}
}

func TestNativeCOFFExactMatchAndNewestCalls(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("COFF execution needs Windows")
	}
	needNative(t)
	target := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH] + "-pc-windows-msvc"
	code := "movl $42, %eax\nret\n"
	if runtime.GOARCH == "arm64" {
		code = "mov w0, #42\nret\n"
	}
	data, o := coffAssembly(t, target, ".section .text$selected,\"xr\",same_contents,selected\n.globl selected\nselected:\n"+code)
	aux := 0
	for i, v := range o.symbols {
		if v.section == o.groups[0].sections[0] && !v.global {
			aux = int(le.Uint32(data[8:])) + (i+1)*18
			break
		}
	}
	for _, newest := range []bool{false, true} {
		dir := t.TempDir()
		var inputs []string
		for i := 0; i < 2; i++ {
			b := append([]byte(nil), data...)
			le.PutUint32(b[4:], uint32(i+1))
			if newest {
				b[aux+14] = 7
				if i == 0 { // Older definition returns 41.
					sec := o.groups[0].sections[0]
					header := 20 + (sec-1)*40
					position := int(le.Uint32(b[header+20:]))
					if runtime.GOARCH == "arm64" {
						le.PutUint32(b[position:], 0x52800520)
					} else {
						b[position+1] = 41
					}
				}
			}
			path := filepath.Join(dir, []string{"old.obj", "new.obj"}[i])
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			inputs = append(inputs, path)
		}
		for _, paths := range [][]string{inputs, {inputs[1], inputs[0]}} {
			// Both definitions must be selected: unused archive duplicates do
			// not participate, matching ordinary demand-based archive linking.
			s := New(Options{})
			load(t, s, paths...)
			call(t, s, "selected", 0, 0, 42)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

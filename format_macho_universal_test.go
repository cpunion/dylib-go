package dylib

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type universalTestSlice struct {
	cpu, sub uint32
	data     []byte
}

func universalBytes(large bool, slices ...universalTestSlice) []byte {
	width, magic := 20, uint32(0xcafebabe)
	if large {
		width, magic = 32, 0xcafebabf
	}
	b := make([]byte, (8+width*len(slices)+7)&^7)
	be := binary.BigEndian
	be.PutUint32(b, magic)
	be.PutUint32(b[4:], uint32(len(slices)))
	for i, s := range slices {
		h := b[8+i*width : 8+(i+1)*width]
		be.PutUint32(h, s.cpu)
		be.PutUint32(h[4:], s.sub)
		if large {
			be.PutUint64(h[8:], uint64(len(b)))
			be.PutUint64(h[16:], uint64(len(s.data)))
			be.PutUint32(h[24:], 3)
		} else {
			be.PutUint32(h[8:], uint32(len(b)))
			be.PutUint32(h[12:], uint32(len(s.data)))
			be.PutUint32(h[16:], 3)
		}
		b = append(b, s.data...)
		for len(b)&7 != 0 {
			b = append(b, 0)
		}
	}
	return b
}

func universalObjects(t *testing.T) []universalTestSlice {
	t.Helper()
	dir := t.TempDir()
	var slices []universalTestSlice
	for i, triple := range []string{"x86_64-apple-macosx11", "arm64-apple-macosx11"} {
		src := filepath.Join(dir, fmt.Sprintf("add-%d.c", i))
		if err := os.WriteFile(src, []byte(fmt.Sprintf("int add(int a,int b){return a+b+%d;}\n", i+1)), 0600); err != nil {
			t.Fatal(err)
		}
		path := compile(t, src, filepath.Join(dir, fmt.Sprintf("add-%d.o", i)), "--target="+triple, "-ffreestanding")
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		cpu, sub, _, err := machoHeader(b)
		if err != nil {
			t.Fatal(err)
		}
		slices = append(slices, universalTestSlice{cpu, sub, b})
	}
	return slices
}

func TestUniversalMachOInspectionAndSelection(t *testing.T) {
	slices := universalObjects(t)
	for _, large := range []bool{false, true} {
		for _, archive := range []bool{false, true} {
			input := append([]universalTestSlice(nil), slices...)
			kind := "object"
			if archive {
				kind = "archive"
				for i := range input {
					input[i].data = append([]byte("!<arch>\n"), archiveMemberBytes("add.o/", input[i].data)...)
				}
			}
			b := universalBytes(large, input...)
			f, err := parse("universal", b)
			if err != nil {
				t.Fatal(err)
			}
			if f.info.Kind != "universal" || len(f.info.Members) != 2 || len(f.slices) != 2 || len(f.members) != 0 {
				t.Fatalf("universal metadata: %+v", f.info)
			}
			for i, arch := range []string{"amd64", "arm64"} {
				selected, err := f.hostSlice("darwin", arch)
				if err != nil || selected != f.slices[i] || selected.info.Kind != kind || selected.info.Bits != 64 {
					t.Fatalf("%s selection: %+v, %v", arch, selected, err)
				}
			}
			if _, err := f.hostSlice("linux", "amd64"); err == nil {
				t.Fatal("foreign OS selection accepted")
			}
			path := filepath.Join(t.TempDir(), "fat")
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			info, err := Inspect(path)
			if err != nil || len(info.Members) != 2 || info.Members[0].Arch != "amd64" || info.Members[1].Arch != "arm64" {
				t.Fatalf("pure inspection: %+v, %v", info, err)
			}
			if runtime.GOOS != "darwin" {
				s := New(Options{})
				if err := s.Load(path); err == nil || len(s.files) != 0 || len(s.libs) != 0 || s.paths[path] {
					t.Fatalf("foreign host published a slice: %v", err)
				}
				s.Close()
			}
		}
	}
}

func TestUniversalMachOSubtypeSelection(t *testing.T) {
	slices := universalObjects(t)
	arm := slices[1]
	arm.sub = 2 // arm64e is inspectable but has no raw execution support.
	arm.data = append([]byte(nil), arm.data...)
	binary.LittleEndian.PutUint32(arm.data[8:], arm.sub)
	f, err := parse("arm variants", universalBytes(false, arm, slices[1]))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := f.hostSlice("darwin", "arm64")
	if err != nil || selected.info.CPUSubtype != 0 {
		t.Fatalf("baseline selection: %+v, %v", selected, err)
	}
	f.slices = f.slices[:1]
	if _, err := f.hostSlice("darwin", "arm64"); err == nil || !strings.Contains(err.Error(), "no baseline") {
		t.Fatalf("authenticated subtype accepted: %v", err)
	}
	// The OS chooses dylib variants itself, so do not validate one variant
	// and then permit the OS to open a different, unvalidated host variant.
	f.slices = append(f.slices, selected)
	selected.info.Kind = "shared"
	if _, err := f.hostSlice("darwin", "arm64"); err == nil || !strings.Contains(err.Error(), "multiple host CPU variants") {
		t.Fatalf("ambiguous OS library selection: %v", err)
	}
}

func TestMalformedUniversalMachO(t *testing.T) {
	slices := universalObjects(t)
	be := binary.BigEndian
	for _, large := range []bool{false, true} {
		base := universalBytes(large, slices...)
		for _, patch := range []func([]byte) []byte{
			func(b []byte) []byte { return b[:7] },
			func(b []byte) []byte { be.PutUint32(b[4:], 0); return b },
			func(b []byte) []byte { be.PutUint32(b[4:], 65); return b },
			func(b []byte) []byte { be.PutUint32(b[8:], slices[1].cpu); return b },
			func(b []byte) []byte {
				width := 20
				if large {
					width = 32
				}
				copy(b[8+width:8+width+8], b[8:16])
				return b
			},
			func(b []byte) []byte {
				if large {
					be.PutUint64(b[16:], 0)
				} else {
					be.PutUint32(b[16:], 0)
				}
				return b
			},
			func(b []byte) []byte {
				if large {
					be.PutUint64(b[16:], ^uint64(0))
				} else {
					be.PutUint32(b[16:], ^uint32(0))
				}
				return b
			},
			func(b []byte) []byte {
				if large {
					be.PutUint32(b[32:], 64)
				} else {
					be.PutUint32(b[24:], 64)
				}
				return b
			},
			func(b []byte) []byte {
				if large {
					be.PutUint64(b[24:], ^uint64(0))
				} else {
					be.PutUint32(b[20:], ^uint32(0))
				}
				return b
			},
			func(b []byte) []byte {
				if large {
					copy(b[48:64], b[16:32])
				} else {
					copy(b[36:44], b[16:24])
				}
				return b
			},
		} {
			if _, err := parse("bad universal", patch(append([]byte(nil), base...))); err == nil {
				t.Fatal("malformed universal container accepted")
			}
		}
		if large {
			b := append([]byte(nil), base...)
			be.PutUint32(b[36:], 1)
			if _, err := parse("reserved bits", b); err == nil {
				t.Fatal("reserved fat64 bits accepted")
			}
		}
	}
	wrong := append([]universalTestSlice(nil), slices...)
	wrong[0].data = append([]byte("!<arch>\n"), archiveMemberBytes("arm.o/", slices[1].data)...)
	if _, err := parse("wrong archive CPU", universalBytes(false, wrong...)); err == nil {
		t.Fatal("archive target mismatch accepted")
	}
}

func TestNativeUniversalMachOObjectsAndArchives(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O execution requires macOS")
	}
	needNative(t)
	slices := universalObjects(t)
	for _, large := range []bool{false, true} {
		for _, archive := range []bool{false, true} {
			input := append([]universalTestSlice(nil), slices...)
			if archive {
				for i := range input {
					input[i].data = append([]byte("!<arch>\n"), archiveMemberBytes("add.o/", input[i].data)...)
				}
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "universal")
			var paths []string
			for i, slice := range input {
				p := filepath.Join(dir, fmt.Sprintf("slice-%d", i))
				if err := os.WriteFile(p, slice.data, 0600); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, p)
			}
			args := []string{"-create", paths[0], paths[1], "-output", path}
			if large {
				args = append(args, "-fat64")
			}
			command(t, "lipo", args...)
			s := New(Options{})
			load(t, s, path)
			want := int32(43)
			if runtime.GOARCH == "arm64" {
				want = 44
			}
			call(t, s, "add", 20, 22, want)
			s.Close()
		}
	}
}

func TestNativeUniversalMachOLipoLibrary(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("universal dylibs require macOS")
	}
	needNative(t)
	dir := t.TempDir()
	sdk, err := exec.Command("xcrun", "--show-sdk-path").Output()
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for i, arch := range []string{"x86_64", "arm64"} {
		src, path := filepath.Join(dir, arch+".c"), filepath.Join(dir, arch+".dylib")
		if err := os.WriteFile(src, []byte(fmt.Sprintf("int add(int a,int b){return a+b+%d;}\n", i+1)), 0600); err != nil {
			t.Fatal(err)
		}
		command(t, compiler(), "-arch", arch, "-dynamiclib", "-Wl,-syslibroot,"+strings.TrimSpace(string(sdk)), src, "-o", path)
		paths = append(paths, path)
	}
	path := filepath.Join(dir, "universal.dylib")
	command(t, "lipo", "-create", paths[0], paths[1], "-output", path)
	i, err := Inspect(path)
	if err != nil || i.Kind != "universal" || len(i.Members) != 2 || i.Members[0].Kind != "shared" {
		t.Fatalf("lipo library: %+v, %v", i, err)
	}
	s := New(Options{})
	defer s.Close()
	load(t, s, path)
	want := int32(43)
	if runtime.GOARCH == "arm64" {
		want = 44
	}
	call(t, s, "add", 20, 22, want)
}

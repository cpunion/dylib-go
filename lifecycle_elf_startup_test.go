package dylib

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func startupInputs(t *testing.T, triple string) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for _, part := range []string{"0", "1", "2"} {
		path := filepath.Join(dir, "part"+part+".o")
		command(t, compiler(), "--target="+triple, "-DPART="+part, "-c", "testdata/lifecycle_startup.S", "-o", path)
		paths = append(paths, path)
	}
	return paths
}
func startupObjects(t *testing.T, paths []string) []*object {
	t.Helper()
	var objects []*object
	for i, path := range paths {
		data, err := readFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parse(path, data)
		if err != nil || len(f.info.Unsupported) != 0 {
			t.Fatal(f, err)
		}
		f.obj.inputOrder = i
		objects = append(objects, f.obj)
	}
	return objects
}
func TestELFStartupFragmentLayout(t *testing.T) {
	for _, triple := range []string{"x86_64-linux-gnu", "aarch64-linux-gnu", "i686-linux-gnu"} {
		t.Run(triple, func(t *testing.T) {
			objects := startupObjects(t, startupInputs(t, triple))
			// Simulate late archive extraction without changing input precedence.
			objects[1], objects[2] = objects[2], objects[1]
			defs, err := definitions(objects)
			if err != nil {
				t.Fatal(err)
			}
			im := &image{objects: objects, defs: defs, page: 4096}
			size, err := im.layoutStartup(4096)
			if err != nil || len(im.startup) != 2 || size != 12288 {
				t.Fatal(size, im.startup, err)
			}
			im.mem = make([]byte, size)
			if err = im.fillStartupPadding(); err != nil {
				t.Fatal(err)
			}
			for _, group := range im.startup {
				if group.parts[0].object.inputOrder != 0 || group.parts[1].object.inputOrder != 1 || group.parts[2].object.inputOrder != 2 {
					t.Fatal("extraction changed fragment order")
				}
				for i := 1; i < len(group.parts); i++ {
					prior, next := group.parts[i-1].section, group.parts[i].section
					gap := im.mem[prior.offset+prior.size : next.offset]
					if len(gap) == 0 {
						t.Fatal("fixture failed to exercise alignment padding")
					}
					for j := 0; j < len(gap); j++ {
						if group.arch == "arm64" {
							if j%4 == 0 && binary.LittleEndian.Uint32(gap[j:]) != 0xd503201f {
								t.Fatal("non-NOP startup padding")
							}
						} else if gap[j] != 0x90 {
							t.Fatal("non-NOP startup padding")
						}
					}
				}
			}
			// Missing epilogue or entry must fail before allocation/initialization.
			for _, bad := range [][]*object{{objects[0], objects[2]}, {objects[1], objects[2]}} {
				defs, _ := definitions(bad)
				image := &image{objects: bad, defs: defs, page: 4096}
				if _, err = image.layoutStartup(0); err == nil {
					t.Fatal("incomplete startup accepted")
				}
			}
			// Members of a single archive also retain member order when
			// their symbol-triggered discovery order differs.
			for _, object := range objects {
				object.memberOrder, object.inputOrder = object.inputOrder, 0
			}
			im = &image{objects: objects, defs: defs, page: 4096}
			if _, err = im.layoutStartup(0); err != nil {
				t.Fatal(err)
			}
			for _, group := range im.startup {
				for index, part := range group.parts {
					if part.object.memberOrder != index {
						t.Fatal("extraction changed archive member order")
					}
				}
			}
		})
	}
}
func TestELFStartupSectionValidation(t *testing.T) {
	for _, declaration := range []string{`.section .init,"aw",@progbits
.byte 0`, `.section .fini,"ax",@nobits
.zero 8`} {
		p := filepath.Join(t.TempDir(), "bad.s")
		if err := os.WriteFile(p, []byte(declaration), 0600); err != nil {
			t.Fatal(err)
		}
		command(t, compiler(), "--target=x86_64-linux-gnu", "-c", p, "-o", p+".o")
		if _, err := Inspect(p + ".o"); err == nil {
			t.Fatal("invalid startup section accepted")
		}
	}
}

func TestELFStartupFrameVariants(t *testing.T) {
	armBegin := []byte{0xfd, 0x7b, 0xbf, 0xa9}
	armEnd := []byte{0xfd, 0x7b, 0xc1, 0xa8, 0xc0, 0x03, 0x5f, 0xd6}
	pacBegin := append([]byte{0x3f, 0x23, 0x03, 0xd5}, armBegin...)
	pacEnd := []byte{0xfd, 0x7b, 0xc1, 0xa8, 0xbf, 0x23, 0x03, 0xd5, 0xc0, 0x03, 0x5f, 0xd6}
	for _, tc := range []struct {
		name, arch string
		begin, end []byte
		valid      bool
	}{
		{"cet64", "amd64", []byte{0xf3, 0x0f, 0x1e, 0xfa, 0x48, 0x83, 0xec, 8}, []byte{0x48, 0x83, 0xc4, 8, 0xc3}, true},
		{"cet32", "386", []byte{0xf3, 0x0f, 0x1e, 0xfb, 0x53, 0x83, 0xec, 8}, []byte{0x83, 0xc4, 8, 0x5b, 0xc3}, true},
		{"arm64", "arm64", armBegin, armEnd, true},
		{"glibc_nop", "arm64", append([]byte{0x1f, 0x20, 0x03, 0xd5}, armBegin...), armEnd, true},
		{"pac", "arm64", pacBegin, pacEnd, true},
		{"bti_pac", "arm64", append([]byte{0x5f, 0x24, 0x03, 0xd5}, pacBegin...), pacEnd, true},
		{"missing_auth", "arm64", pacBegin, armEnd, false},
		{"missing_sign", "arm64", armBegin, pacEnd, false},
		{"missing_return", "arm64", armBegin, armEnd[:4], false},
		{"wrong_x86_frame", "amd64", []byte{0x48, 0x83, 0xec, 16}, []byte{0x48, 0x83, 0xc4, 8, 0xc3}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := standardStartupFrame(tc.arch, tc.begin, tc.end); got != tc.valid {
				t.Fatalf("complete frame = %v, want %v", got, tc.valid)
			}
		})
	}
}
func TestNativeELFStartupFragments(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF execution requires Linux")
	}
	needNative(t)
	triple := map[string]string{"amd64": "x86_64-linux-gnu", "arm64": "aarch64-linux-gnu", "386": "i686-linux-gnu"}[runtime.GOARCH]
	parts := startupInputs(t, triple)
	dir := t.TempDir()
	body := compile(t, "testdata/lifecycle_startup.c", filepath.Join(dir, "body.o"))
	archive := filepath.Join(dir, "startup.a")
	command(t, "ar", "rcs", archive, parts[1], body)
	for _, inputs := range [][]string{{parts[0], parts[1], body, parts[2]}, {parts[0], archive, parts[2]}} {
		t.Run(filepath.Base(inputs[1]), func(t *testing.T) {
			checkStartupLifecycle(t, inputs)
		})
	}
	t.Run("without_arrays", func(t *testing.T) {
		body := compile(t, "testdata/lifecycle_startup.c", filepath.Join(t.TempDir(), "body.o"), "-DNO_ARRAYS")
		host, _, read := lifecycleObserver(t, t.TempDir())
		s := New(Options{})
		defer s.Close()
		load(t, s, host, parts[0], parts[1], body, parts[2])
		call(t, s, "startup_root", 0, 0, 42)
		expectEvents(t, read, 2)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		expectEvents(t, read, 2, 5)
	})
	host, _, read := lifecycleObserver(t, t.TempDir())
	s := New(Options{})
	defer s.Close()
	load(t, s, host, parts[0], parts[1], body)
	if err := s.Link("startup_root"); err == nil || !strings.Contains(err.Error(), "epilogue") {
		t.Fatal("missing CRTN accepted", err)
	}
	expectEvents(t, read)
	load(t, s, parts[2])
	if err := s.Link("startup_root"); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 1, 2, 3)
}

func checkStartupLifecycle(t *testing.T, inputs []string) {
	t.Helper()
	host, _, read := lifecycleObserver(t, t.TempDir())
	s := New(Options{})
	defer s.Close()
	load(t, s, host)
	load(t, s, inputs...)
	expectEvents(t, read)
	if err := s.Link("startup_root", "startup_anchor"); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 1, 2, 3)
	call(t, s, "startup_root", 0, 0, 42)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 1, 2, 3, 4, 5)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	expectEvents(t, read, 1, 2, 3, 4, 5)
}

func TestNativeELFSystemStartupFragments(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("system ELF CRT execution requires Linux")
	}
	needNative(t)
	crt := func(name string) string {
		t.Helper()
		var args []string
		if runtime.GOARCH == "386" {
			args = append(args, "-m32")
		}
		args = append(args, "-print-file-name="+name)
		output, err := exec.Command(compiler(), args...).CombinedOutput()
		path := strings.TrimSpace(string(output))
		if err != nil {
			t.Fatalf("locate %s: %v\n%s", name, err, output)
		}
		if !filepath.IsAbs(path) {
			if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
				t.Fatalf("required system CRT %s unavailable: %s", name, path)
			}
			t.Skipf("system CRT %s unavailable", name)
		}
		return path
	}
	prefix, suffix := crt("crti.o"), crt("crtn.o")
	triple := map[string]string{"amd64": "x86_64-linux-gnu", "arm64": "aarch64-linux-gnu", "386": "i686-linux-gnu"}[runtime.GOARCH]
	parts := startupInputs(t, triple)
	dir := t.TempDir()
	body := compile(t, "testdata/lifecycle_startup.c", filepath.Join(dir, "body.o"))
	archive := filepath.Join(dir, "startup.a")
	command(t, "ar", "rcs", archive, parts[1], body)
	for _, inputs := range [][]string{{prefix, parts[1], body, suffix}, {prefix, archive, suffix}} {
		t.Run(filepath.Base(inputs[1]), func(t *testing.T) { checkStartupLifecycle(t, inputs) })
	}
}

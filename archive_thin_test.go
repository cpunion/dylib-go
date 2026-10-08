package dylib

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type thinTestMember struct {
	name   string
	size   uint64
	origin uint64
}

func thinHeader(name string, size uint64) []byte {
	return []byte(fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", name, "0", "0", "0", "644", size))
}

func thinBytes(members ...thinTestMember) []byte {
	var names []byte
	var headers []byte
	for _, member := range members {
		reference := fmt.Sprintf("/%d", len(names))
		if member.origin != 0 {
			reference += fmt.Sprintf(":%d", member.origin)
		}
		names = append(names, []byte(filepath.ToSlash(member.name)+"/\n")...)
		headers = append(headers, thinHeader(reference, member.size)...)
	}
	b := append([]byte("!<thin>\n"), archiveMemberBytes("//", names)...)
	return append(b, headers...)
}

func writeThin(t *testing.T, path string, members ...thinTestMember) {
	t.Helper()
	if err := os.WriteFile(path, thinBytes(members...), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestThinArchiveInspectionAndProxyMembers(t *testing.T) {
	dir := t.TempDir()
	p := compile(t, "testdata/add.c", filepath.Join(dir, "long object name.obj"), "--target=x86_64-w64-windows-gnu")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	regular := append([]byte("!<arch>\n"), archiveMemberBytes("//", []byte("long object name.obj/\n"))...)
	offset := uint64(len(regular))
	regular = append(regular, archiveMemberBytes("/0", b)...)
	inner := filepath.Join(dir, "inner archive.a")
	if err := os.WriteFile(inner, regular, 0600); err != nil {
		t.Fatal(err)
	}
	outer := filepath.Join(dir, "outer.a")
	writeThin(t, outer,
		thinTestMember{name: filepath.Base(p), size: 1}, // Size hints can be stale.
		thinTestMember{name: p, size: uint64(len(b))},
		thinTestMember{name: filepath.Base(inner), size: uint64(len(b)), origin: offset})
	i, err := Inspect(outer)
	if err != nil {
		t.Fatal(err)
	}
	if !i.Thin || i.Kind != "archive" || i.Format != "ar" || len(i.Members) != 3 {
		t.Fatalf("thin metadata: %+v", i)
	}
	for _, member := range i.Members {
		if member.Format != "COFF" || member.Arch != "amd64" || len(member.Symbols) == 0 || member.Symbols[0].Name != "add" {
			t.Fatalf("foreign object/proxy metadata: %+v", member)
		}
	}
	if !strings.Contains(i.Members[2].Name, "long object name.obj") {
		t.Fatal("proxy inspection lost the actual member name")
	}
	if f, err := parse("empty.a", []byte("!<thin>\n")); err != nil || !f.info.Thin || len(f.members) != 0 {
		t.Fatalf("empty thin archive: %+v, %v", f, err)
	}
	// Relative origins belong to the referenced object/archive, including DLLs.
	other := filepath.Join(dir, "archives")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	outer = filepath.Join(other, "origins.a")
	writeThin(t, outer, thinTestMember{name: "../long object name.obj"}, thinTestMember{name: "../inner archive.a", origin: offset})
	f, err := parse(outer, thinBytes(thinTestMember{name: "../long object name.obj"}, thinTestMember{name: "../inner archive.a", origin: offset}))
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range f.members {
		if member.obj.directory != dir {
			t.Fatalf("external origin = %s, want %s", member.obj.directory, dir)
		}
	}
}

func thinTool(t *testing.T) string {
	t.Helper()
	candidates := []string{"llvm-ar"}
	if runtime.GOOS != "darwin" {
		candidates = append(candidates, "ar")
	}
	// Homebrew LLVM tools can be keg-only on Go CI's macOS runners.
	for _, pattern := range []string{"/opt/homebrew/opt/llvm*/bin/llvm-ar", "/usr/local/opt/llvm*/bin/llvm-ar"} {
		paths, _ := filepath.Glob(pattern)
		candidates = append(candidates, paths...)
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	if os.Getenv("DYLIB_TEST_REQUIRE_TOOLS") == "1" {
		t.Fatal("a GNU/LLVM thin archiver is required")
	}
	t.Skip("GNU/LLVM thin archiver unavailable")
	return ""
}

func TestNativeThinArchiveExtractionAndSnapshot(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	objects := filepath.Join(dir, "objects with spaces")
	if err := os.Mkdir(objects, 0700); err != nil {
		t.Fatal(err)
	}
	add := compile(t, "testdata/add.c", filepath.Join(objects, "long add name.o"))
	unused := compile(t, "testdata/missing.c", filepath.Join(objects, "unused.o"))
	caller := compile(t, "testdata/caller.c", filepath.Join(dir, "caller.o"))
	p := filepath.Join(dir, "plugins.a")
	cmd := exec.Command(thinTool(t), "rcsT", p, "objects with spaces/long add name.o", "objects with spaces/unused.o")
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("thin archiver: %v\n%s", err, b)
	}
	i, err := Inspect(p)
	if err != nil || !i.Thin || len(i.Members) != 2 {
		t.Fatalf("genuine thin archive: %+v, %v", i, err)
	}
	s := New(Options{})
	defer s.Close()
	load(t, s, p, caller)
	for _, member := range s.files[0].members {
		if member.obj.directory != objects {
			t.Fatal("Load replaced the external object's dependency origin")
		}
	}
	// No member is reread during Link, and unused missing dependencies stay unused.
	for _, path := range []string{add, unused, p} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	call(t, s, "caller", 20, 22, 43)
}

func TestNativeThinArchiveRegularMemberProxy(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	p := compile(t, "testdata/add.c", filepath.Join(dir, "add.o"))
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(dir, "inner.a")
	if err := os.WriteFile(inner, append([]byte("!<arch>\n"), archiveMemberBytes("add.o/", b)...), 0600); err != nil {
		t.Fatal(err)
	}
	outer := filepath.Join(dir, "outer.a")
	writeThin(t, outer, thinTestMember{name: "inner.a", size: uint64(len(b)), origin: 8})
	s := New(Options{})
	defer s.Close()
	load(t, s, outer)
	// GNU ar emits the proxy syntax when a regular archive is added to a thin
	// archive. Other archivers can instead retain a non-object archive member.
	if tool, err := exec.LookPath("ar"); err == nil {
		version, _ := exec.Command(tool, "--version").CombinedOutput()
		if strings.Contains(string(version), "GNU ar") {
			genuine := filepath.Join(dir, "gnu.a")
			command(t, tool, "rcsT", genuine, inner)
			g := New(Options{})
			defer g.Close()
			load(t, g, genuine)
			call(t, g, "add", 20, 22, 42)
		}
	}
	if err := os.Remove(inner); err != nil {
		t.Fatal(err)
	}
	call(t, s, "add", 20, 22, 42)
}

func TestThinArchiveFailureAndRetry(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "retry.a")
	writeThin(t, p, thinTestMember{name: "missing.obj"})
	s := New(Options{})
	defer s.Close()
	if err := s.Load(p); err == nil || !strings.Contains(err.Error(), "external member") || len(s.files) != 0 || s.paths[p] {
		t.Fatalf("failed read changed staging: %v", err)
	}
	compile(t, "testdata/add.c", filepath.Join(dir, "missing.obj"), "--target=x86_64-w64-windows-gnu")
	if err := s.Load(p); err != nil || len(s.files) != 1 || !s.paths[p] {
		t.Fatalf("retry: %v", err)
	}
}

func TestThinArchiveLongImportOrigins(t *testing.T) {
	dir := t.TempDir()
	parts := longImportParts(t, "x86_64-pc-windows-msvc", "provider.dll", longImportFixture{public: "add", export: "add"})
	var members []thinTestMember
	for i, data := range parts {
		path := filepath.Join(dir, fmt.Sprintf("import-%d.obj", i))
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		members = append(members, thinTestMember{name: path, size: uint64(len(data))})
	}
	p := filepath.Join(t.TempDir(), "imports.a")
	f, err := parse(p, thinBytes(members...))
	if err != nil {
		t.Fatal(err)
	}
	imported := f.members[len(f.members)-1]
	if len(imported.info.Imports) != 1 || imported.info.Imports[0].Name != "add" || imported.obj.directory != dir {
		t.Fatalf("converted import origin/metadata: %+v, %s", imported.info, imported.obj.directory)
	}
}

func TestNativeThinArchiveDLLOrigins(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DLL imports require Windows")
	}
	needNative(t)
	dll, _, _ := nativeImportFixture(t)
	dir := filepath.Dir(dll)
	machine := map[string]uint16{"amd64": 0x8664, "arm64": 0xaa64, "386": 0x14c}[runtime.GOARCH]
	short := shortImport(machine, 0, 1, "add", "provider.dll", "", 0)
	obj := filepath.Join(dir, "short.obj")
	inner := filepath.Join(dir, "short.a")
	for path, data := range map[string][]byte{obj: short, inner: append([]byte("!<arch>\n"), archiveMemberBytes("short.obj/", short)...)} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	target := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH] + "-pc-windows-msvc"
	parts := longImportParts(t, target, "provider.dll", longImportFixture{public: "add", export: "add"})
	var long []thinTestMember
	for i, data := range parts {
		path := filepath.Join(dir, fmt.Sprintf("long-%d.obj", i))
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		long = append(long, thinTestMember{name: path, size: uint64(len(data))})
	}
	for _, members := range [][]thinTestMember{{{name: obj}}, {{name: inner, origin: 8}}, long} {
		p := filepath.Join(t.TempDir(), "imports.a")
		writeThin(t, p, members...)
		s := New(Options{})
		load(t, s, p)
		call(t, s, "add", 20, 22, 42)
		if len(s.libs) != 1 {
			t.Fatal("external member did not retain its DLL")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMalformedThinArchives(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "inner.a")
	if err := os.WriteFile(inner, append([]byte("!<arch>\n"), archiveMemberBytes("//", []byte("name.o/\n"))...), 0600); err != nil {
		t.Fatal(err)
	}
	nameTable := archiveMemberBytes("//", []byte("inner.a/\n"))
	for _, tc := range []struct{ reference, want string }{
		{"/0:7", "invalid thin archive member offset"},
		{"/0:9", "invalid thin archive member offset"},
		{"/0:bad", "invalid thin archive member offset"},
		{"/0:8:8", "invalid thin archive member offset"},
		{"/0:8", "not an object member header"},
		{"/0:1000", "not an object member header"},
		{"/0:999999999998", "not an object member header"},
		{"/0", "nested archives"},
		{"#1/8", "BSD extended names"},
	} {
		t.Run(tc.reference, func(t *testing.T) {
			b := append([]byte("!<thin>\n"), nameTable...)
			b = append(b, thinHeader(tc.reference, 1)...)
			if _, err := parse(filepath.Join(dir, "bad.a"), b); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
		})
	}
	// Reject malformed trailing headers before reading an earlier missing file.
	b := append(thinBytes(thinTestMember{name: "missing.obj"}), 'x')
	if _, err := parse(filepath.Join(dir, "bad.a"), b); err == nil || !strings.Contains(err.Error(), "bad archive header") {
		t.Fatalf("container validation before I/O: %v", err)
	}
	self := filepath.Join(dir, "self.a")
	writeThin(t, self, thinTestMember{name: "self.a"})
	if _, err := Inspect(self); err == nil || !strings.Contains(err.Error(), "nested archives") {
		t.Fatalf("self-reference: %v", err)
	}
	r := thinReader{remaining: 4, sources: make(map[string]*thinSource)}
	if _, err := r.read(inner); err == nil || !strings.Contains(err.Error(), "at most 4 bytes") || len(r.sources) != 0 || r.remaining != 4 {
		t.Fatalf("read budget before allocation: %v", err)
	}
	r.remaining = maxFile
	first, err := r.read(inner)
	if err != nil {
		t.Fatal(err)
	}
	remaining := r.remaining
	if err := os.Remove(inner); err != nil {
		t.Fatal(err)
	}
	if again, err := r.read(inner); err != nil || again != first || r.remaining != remaining {
		t.Fatalf("external snapshot charged/read more than once: %v", err)
	}
}

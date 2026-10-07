package dylib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDuplicateAndUnknownSymbol(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	a := compile(t, "testdata/add.c", filepath.Join(dir, "a.o"))
	b := compile(t, "testdata/add.c", filepath.Join(dir, "b.o"))
	s := New(Options{})
	defer s.Close()
	load(t, s, a, b)
	if _, e := s.Lookup("add"); e == nil || !strings.Contains(e.Error(), "duplicate strong") {
		t.Fatalf("duplicate: %v", e)
	}
	s2 := New(Options{})
	defer s2.Close()
	load(t, s2, a)
	if _, e := s2.Lookup("unknown"); e == nil {
		t.Fatal("unknown symbol accepted")
	}
	call(t, s2, "add", 20, 22, 42)
	if e := s2.Link("unknown"); e == nil {
		t.Fatal("new root after linking reported success")
	}
	if e := s2.Link("add\x00unknown"); e == nil {
		t.Fatal("NUL root accepted")
	}
}

func TestMalformedArchives(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("!<arch>\nx"), []byte("!<thin>\n"), append([]byte("!<arch>\n"), make([]byte, 60)...)} {
		if _, e := parse("bad", b); e == nil {
			t.Fatal("malformed input accepted")
		}
	}
	b := []byte("!<arch>\n" + fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", "nested.a/", "0", "0", "0", "644", 8) + "!<arch>\n")
	if _, e := parse("nested", b); e == nil || !strings.Contains(e.Error(), "nested archives") {
		t.Fatalf("nested: %v", e)
	}
}

func TestRelocationFailureDoesNotPublishImage(t *testing.T) {
	needNative(t)
	dir := t.TempDir()
	p := compile(t, "testdata/caller.c", filepath.Join(dir, "caller.o"))
	add := compile(t, "testdata/add.c", filepath.Join(dir, "add.o"))
	s := New(Options{})
	defer s.Close()
	load(t, s, p, add)
	o := s.files[0].obj
	if len(o.relocs) == 0 {
		t.Fatal("fixture missing relocation")
	}
	old := o.relocs[0]
	o.relocs[0].offset = ^uint64(0)
	if _, e := s.Lookup("caller"); e == nil || !strings.Contains(e.Error(), "outside section") {
		t.Fatalf("bounds: %v", e)
	}
	if s.image != nil {
		t.Fatal("failed image published")
	}
	o.relocs[0] = old
	o.relocs[0].typ = 0xffff
	if _, e := s.Lookup("caller"); e == nil || !strings.Contains(e.Error(), "unsupported") {
		t.Fatalf("unsupported relocation: %v", e)
	}
	o.relocs[0] = old
	call(t, s, "caller", 20, 22, 43)
}

func TestApplePlatformBoundary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "ios.c")
	if e := os.WriteFile(src, []byte("int add(int a,int b){return a+b;}"), 0600); e != nil {
		t.Fatal(e)
	}
	p := compile(t, src, filepath.Join(dir, "ios.o"), "--target=arm64-apple-ios18")
	i, e := Inspect(p)
	if e != nil {
		t.Fatal(e)
	}
	if i.OS == "darwin" || i.OS == "" {
		t.Fatalf("iOS platform lost: %+v", i)
	}
	s := New(Options{})
	defer s.Close()
	if e = s.Load(p); e == nil {
		t.Fatal("iOS object accepted by desktop host")
	}
}

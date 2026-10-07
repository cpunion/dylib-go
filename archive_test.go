package dylib

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func archiveMemberBytes(name string, contents []byte) []byte {
	header := fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", name, "0", "0", "0", "644", len(contents))
	b := append([]byte(header), contents...)
	if len(contents)%2 != 0 {
		b = append(b, '\n')
	}
	return b
}

func TestArchiveLongNameFormats(t *testing.T) {
	// Parse a real COFF object on every test host. This test needs no native
	// execution backend and exercises both offset zero and later table entries.
	p := compile(t, "testdata/add.c", filepath.Join(t.TempDir(), "add.obj"), "--target=x86_64-w64-windows-gnu")
	obj, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	first := "a_very_long_archive_member_name.obj"
	second := "another_long_archive_member_name.obj"
	for _, tc := range []struct {
		name, terminator string
	}{
		{"GNU", "/\n"},
		{"COFF", "\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			names := []byte(first + tc.terminator + second + tc.terminator)
			b := append([]byte("!<arch>\n"), archiveMemberBytes("//", names)...)
			b = append(b, archiveMemberBytes("/0", obj)...)
			b = append(b, archiveMemberBytes("/"+strconv.Itoa(len(first)+len(tc.terminator)), obj)...)
			f, err := parse("names.lib", b)
			if err != nil {
				t.Fatal(err)
			}
			if len(f.info.Members) != 2 {
				t.Fatalf("got %d members, want 2", len(f.info.Members))
			}
			for i, want := range []string{first, second} {
				member := f.info.Members[i]
				if member.Name != "names.lib("+want+")" || member.Format != "COFF" {
					t.Fatalf("member %d: %+v", i, member)
				}
			}
		})
	}
}

func TestMalformedArchiveLongNames(t *testing.T) {
	for _, tc := range []struct {
		name, table, reference, want string
	}{
		{"missing-table", "", "/0", "invalid archive name offset"},
		{"negative", "name\x00", "/-1", "invalid archive name offset"},
		{"at-end", "name\x00", "/5", "invalid archive name offset"},
		{"huge-offset", "name\x00", "/999999999999999", "invalid archive name offset"},
		{"non-numeric", "name\x00", "/bad", "invalid archive name offset"},
		{"unterminated", "name", "/0", "unterminated archive name"},
		{"empty-COFF", "\x00", "/0", "empty archive member name"},
		{"empty-GNU", "/\n", "/0", "empty archive member name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := append([]byte("!<arch>\n"), archiveMemberBytes("//", []byte(tc.table))...)
			b = append(b, archiveMemberBytes(tc.reference, nil)...)
			if _, err := parse("bad.lib", b); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

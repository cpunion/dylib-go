package llvm

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	dylib "github.com/cpunion/dylib-go"
	"github.com/cpunion/dylib-go/internal/ar"
)

type thinReference struct {
	name   string
	offset uint64
}

func thinArchive(t *testing.T, path string, references ...thinReference) {
	t.Helper()
	var table, headers []byte
	header := func(name string, size int) []byte {
		return []byte(fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", name, "0", "0", "0", "644", size))
	}
	for _, reference := range references {
		name := fmt.Sprintf("/%d", len(table))
		if reference.offset != 0 {
			name += fmt.Sprintf(":%d", reference.offset)
		}
		headers = append(headers, header(name, 1)...)
		table = append(table, []byte(filepath.ToSlash(reference.name)+"/\n")...)
	}
	data := append([]byte("!<thin>\n"), header("//", len(table))...)
	data = append(data, table...)
	if len(table)&1 != 0 {
		data = append(data, '\n')
	}
	data = append(data, headers...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestThinNativeArchivesSourcesProxiesAndSnapshots(t *testing.T) {
	dir := t.TempDir()
	first, second, outer := filepath.Join(dir, "objects with spaces"), filepath.Join(dir, "inner archives"), filepath.Join(dir, "containers")
	for _, folder := range []string{first, second, outer} {
		if err := os.Mkdir(folder, 0700); err != nil {
			t.Fatal(err)
		}
	}
	object := filepath.Join(first, "native.o")
	if err := os.WriteFile(object, emptyCOFF(), 0600); err != nil {
		t.Fatal(err)
	}
	selected := emptyCOFF()
	binary.LittleEndian.PutUint32(selected[4:], 42)
	inner := filepath.Join(second, "inner.a")
	data := ordinaryArchive(t, inner, []string{"first.o", "picked member.o"}, [][]byte{emptyCOFF(), selected})
	entries, err := ar.Decode(data, false, maxInputSize, maxArchiveMembers)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(outer, "thin.a")
	thinArchive(t, path,
		thinReference{name: "../objects with spaces/native.o"},
		thinReference{name: object},
		thinReference{name: "../inner archives/inner.a", offset: entries[1].Offset})
	archive, err := CompileArchive(context.Background(), path, Options{Compiler: filepath.Join(dir, "no compiler"), TempDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if !reflect.DeepEqual(archive.SourceDirectories, []string{first, second, outer}) {
		t.Fatalf("source directories: %v", archive.SourceDirectories)
	}
	for _, source := range []string{object, inner, path} {
		if err := os.Remove(source); err != nil {
			t.Fatal(err)
		}
	}
	info, err := dylib.Inspect(archive.Path)
	if err != nil || info.Thin || len(info.Members) != 3 || !strings.Contains(info.Members[2].Name, "picked member.o") {
		t.Fatalf("compiled thin/proxy snapshots: %+v, %v", info, err)
	}
	output, err := os.ReadFile(archive.Path)
	if err != nil {
		t.Fatal(err)
	}
	native, err := ar.Decode(output, false, maxInputSize, maxArchiveMembers)
	if err != nil || !bytes.Equal(native[2].Data, selected) {
		t.Fatalf("proxy did not select the requested object: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{first, second, outer} {
		if _, err := os.Stat(folder); err != nil {
			t.Fatal("Close removed original directory:", err)
		}
	}
	empty := filepath.Join(outer, "empty.a")
	thinArchive(t, empty)
	archive, err = CompileArchive(context.Background(), empty, Options{})
	if err != nil || archive.Info.Thin || len(archive.Info.Members) != 0 {
		t.Fatalf("empty thin conversion: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestThinArchiveFailureBeforeCompilerStorage(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.o")
	if err := os.WriteFile(valid, emptyCOFF(), 0600); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(dir, "inner.a")
	ordinaryArchive(t, inner, []string{"valid.o"}, [][]byte{emptyCOFF()})
	path := filepath.Join(dir, "bad.a")
	for _, test := range []struct {
		reference thinReference
		want      string
	}{
		{thinReference{name: "missing.o"}, "external member"},
		{thinReference{name: "inner.a"}, "nested archives"},
		{thinReference{name: "inner.a", offset: 9}, "invalid thin archive member offset"},
		{thinReference{name: "inner.a", offset: 100}, "not an object member header"},
		{thinReference{name: "bad.a"}, "nested archives"},
	} {
		thinArchive(t, path, thinReference{name: "valid.o"}, test.reference)
		if _, err := CompileArchive(context.Background(), path, Options{TempDir: dir}); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("thin failure %q: %v", test.want, err)
		}
		leaked, err := filepath.Glob(filepath.Join(dir, "dylib-go-llvm-ar-*"))
		if err != nil || len(leaked) != 0 {
			t.Fatalf("failed snapshot published compiler storage: %v, %v", leaked, err)
		}
	}
	thinArchive(t, path, thinReference{name: "missing.o"})
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Write([]byte{'x'})
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileArchive(context.Background(), path, Options{}); err == nil || !strings.Contains(err.Error(), "bad archive header") {
		t.Fatalf("header validation happened after external I/O: %v", err)
	}
}

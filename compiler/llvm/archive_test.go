package llvm

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cpunion/dylib-go/internal/ar"
)

func ordinaryArchive(t *testing.T, path string, names []string, members [][]byte) []byte {
	t.Helper()
	var data bytes.Buffer
	w := archiveWriter{output: &data, remaining: maxInputSize}
	if err := w.write([]byte("!<arch>\n")); err != nil {
		t.Fatal(err)
	}
	for i, name := range names {
		if err := w.member(name, members[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func emptyCOFF() []byte {
	data := make([]byte, 96) // debug/pe initially probes a complete DOS header.
	binary.LittleEndian.PutUint16(data, 0x8664)
	return data
}

func TestNativeOnlyArchiveNamesAndOwnership(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.a")
	names := []string{"same.o", "same.o", "../../outside/long member name.o", "odd.o"}
	contents := [][]byte{emptyCOFF(), emptyCOFF(), emptyCOFF(), emptyCOFF()}
	original := ordinaryArchive(t, path, names, contents)
	// Native-only conversion needs no compiler. Every test host can inspect
	// these foreign COFF objects, including pure-Go jobs without LLVM tools.
	archive, err := CompileArchive(context.Background(), path, Options{Compiler: filepath.Join(dir, "no compiler"), TempDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if archive.Info.Kind != "archive" || archive.Info.Thin || len(archive.Info.Members) != len(names) {
		t.Fatalf("native archive metadata: %+v", archive.Info)
	}
	output, err := os.ReadFile(archive.Path)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ar.Decode(output, false, maxInputSize, maxArchiveMembers)
	if err != nil {
		t.Fatal(err)
	}
	for i, entry := range entries {
		if entry.Name != names[i] || !bytes.Equal(entry.Data, contents[i]) {
			t.Fatalf("member %d changed: %q", i, entry.Name)
		}
		if archive.Info.Members[i].Format != "COFF" || archive.Info.Members[i].Arch != "amd64" {
			t.Fatal(archive.Info.Members[i])
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := (*Archive)(nil).Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archive.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output remains after Close: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, original) {
		t.Fatal("compiler changed caller's archive:", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 || files[0].Name() != "input.a" {
		t.Fatalf("owned/extracted files leaked: %v, %v", files, err)
	}
}

func TestArchiveInvalidInputAndRollback(t *testing.T) {
	for _, contents := range [][]byte{nil, []byte("raw IR"), []byte("!<arch>\n"), []byte("!<thin>\n")} {
		t.Run(fmt.Sprintf("member-%x", contents), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "input.a")
			ordinaryArchive(t, path, []string{"valid.o", "invalid"}, [][]byte{emptyCOFF(), contents})
			if _, err := CompileArchive(context.Background(), path, Options{TempDir: dir}); err == nil {
				t.Fatal("invalid/nested archive member accepted")
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 1 {
				t.Fatalf("late failure leaked files: %v, %v", files, err)
			}
		})
	}
	dir := t.TempDir()
	for _, data := range [][]byte{[]byte("unknown"), []byte("!<thin>\n"), []byte("!<arch>\nx")} {
		path := filepath.Join(dir, "bad.a")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := CompileArchive(context.Background(), path, Options{TempDir: dir}); err == nil {
			t.Fatal("invalid container accepted")
		}
	}
	for _, path := range []string{dir, filepath.Join(dir, "missing")} {
		if _, err := CompileArchive(context.Background(), path, Options{}); err == nil {
			t.Fatal("directory/missing input accepted")
		}
	}
	path := filepath.Join(dir, "empty.a")
	ordinaryArchive(t, path, nil, nil)
	if _, err := CompileArchive(nil, path, Options{}); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CompileArchive(ctx, path, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	archive, err := CompileArchive(context.Background(), path, Options{TempDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Info.Members) != 0 {
		t.Fatal("empty archive acquired members")
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveCancellationAndMemberLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "waiting.a")
	ordinaryArchive(t, path, []string{"before.o", "waiting.bc"}, [][]byte{emptyCOFF(), []byte("BC\xc0\xde")})
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DYLIB_LLVM_COMPILER_HELPER", "wait")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := CompileArchive(ctx, path, Options{Compiler: executable, TempDir: dir}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("canceled compilation leaked files: %v, %v", files, err)
	}
	names := make([]string, maxArchiveMembers+1)
	members := make([][]byte, len(names))
	for i := range names {
		names[i] = "empty.o"
	}
	ordinaryArchive(t, path, names, members)
	if _, err := CompileArchive(context.Background(), path, Options{TempDir: dir}); err == nil || !strings.Contains(err.Error(), "object members") {
		t.Fatalf("member count limit: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(maxInputSize + 1)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileArchive(context.Background(), path, Options{TempDir: dir}); err == nil || !strings.Contains(err.Error(), "regular input") {
		t.Fatalf("input size limit: %v", err)
	}
}

func TestArchiveWriterBounds(t *testing.T) {
	var output bytes.Buffer
	w := archiveWriter{output: &output, remaining: 10}
	if err := w.member("name", []byte("payload")); err == nil || output.Len() != 0 {
		t.Fatal("oversized member partially written")
	}
	w.remaining = 1
	if err := w.write([]byte("large")); err == nil || output.Len() != 0 {
		t.Fatal("output byte budget ignored")
	}
	w = archiveWriter{output: shortWriter{}, remaining: 100}
	if err := w.write([]byte("short")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

func TestBitcodeArchiveTargetMetadataAndLateCompilerFailure(t *testing.T) {
	llc := tool(t, "DYLIB_LLC", "llc")
	assembler := tool(t, "DYLIB_LLVM_AS", filepath.Join(filepath.Dir(llc), "llvm-as"))
	for i, target := range objectTargets {
		t.Run(target.triple, func(t *testing.T) {
			dir := t.TempDir()
			ir := writeIR(t, dir, target.triple)
			module := filepath.Join(dir, "module.bc")
			if data, err := exec.Command(assembler, "-o", module, ir).CombinedOutput(); err != nil {
				t.Fatalf("llvm-as: %v\n%s", err, data)
			}
			data, err := os.ReadFile(module)
			if err != nil {
				t.Fatal(err)
			}
			if i%2 != 0 && bytes.HasPrefix(data, []byte("BC\xc0\xde")) {
				// llvm-as already wraps Darwin modules. Wrapping other raw
				// streams must not change the module target either.
				header := make([]byte, 20)
				binary.LittleEndian.PutUint32(header, 0x0b17c0de)
				binary.LittleEndian.PutUint32(header[8:], 20)
				binary.LittleEndian.PutUint32(header[12:], uint32(len(data)))
				binary.LittleEndian.PutUint32(header[16:], 0xffffffff)
				data = append(header, data...)
			}
			input := filepath.Join(dir, "input.a")
			ordinaryArchive(t, input, []string{"module.bc", "foreign-native.o"}, [][]byte{data, emptyCOFF()})
			archive, err := CompileArchive(context.Background(), input, Options{Compiler: llc, TempDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			defer archive.Close()
			member := archive.Info.Members[0]
			if member.Format != target.format || member.Arch != target.arch || len(archive.Info.Members) != 2 {
				t.Fatalf("archive member was retargeted/lost: %+v", archive.Info)
			}
		})
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-module.a")
	ordinaryArchive(t, path, []string{"valid.o", "broken.bc"}, [][]byte{emptyCOFF(), []byte("BC\xc0\xde invalid")})
	if _, err := CompileArchive(context.Background(), path, Options{Compiler: llc, TempDir: dir}); err == nil || !strings.Contains(err.Error(), "broken.bc") {
		t.Fatalf("late compiler error: %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("compiler failure leaked files: %v, %v", files, err)
	}
}

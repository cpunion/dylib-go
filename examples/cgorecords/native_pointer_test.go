//go:build libffi && cgo && (linux || darwin || windows)

package main

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/cpunion/dylib-go/abi"
)

func TestCgoOwnedRecordPointers(t *testing.T) {
	compiler := compilerTool(t)
	object := filepath.Join(t.TempDir(), "exports.o")
	flags := []string{"--target=" + _BindingsDeclarations.Target.Triple, "-O2", "-c", "-fno-stack-protector", "testdata/exports.c", "-o", object}
	if runtime.GOOS != "windows" {
		flags = append(flags, "-fPIC")
	}
	if data, err := exec.Command(compiler, flags...).CombinedOutput(); err != nil {
		t.Fatalf("C producer: %v\n%s", err, data)
	}
	session, err := load(object)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	bindings, err := NewBindings(session)
	if err != nil {
		t.Fatal(err)
	}
	record, err := _BindingsDeclarations.LookupRecord("Pair")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := abi.StructValue(record.Description, abi.Int8(-128), abi.Float64(20.5), abi.Int16(-32767))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := abi.NewNativeValue(record.Description, initial)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	layout, err := owner.Layout()
	if err != nil || !reflect.DeepEqual(layout, record.Layout) {
		t.Fatal("owned storage/Clang layout:", layout, record.Layout, err)
	}
	lease, err := owner.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	pointer, err := lease.Pointer()
	if err != nil {
		t.Fatal(err)
	}
	mutated, err := bindings.Mutate_pair(pointer)
	if err != nil || mutated != pointer {
		t.Fatal("owned struct pointer mutation/identity:", mutated, err)
	}
	want, _ := abi.StructValue(record.Description, abi.Int8(-127), abi.Float64(22), abi.Int16(-32768))
	got, err := lease.Read()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("owned pointee read:", got, want, err)
	}
	checksum, err := bindings.Sum_pair(pointer)
	if err != nil || checksum != -32873 {
		t.Fatal("owned pointee C fields:", checksum, err)
	}
}

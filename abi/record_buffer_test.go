//go:build libffi && cgo

package abi

import (
	"errors"
	"testing"
	"unsafe"
)

func TestRecordBufferReuseZeroingAndIsolation(t *testing.T) {
	desc := TypeDesc{Type: Struct, Fields: []Field{{Name: "tag", Type: TypeDesc{Type: U8}}, {Name: "value", Type: TypeDesc{Type: U64}}}}
	plan, err := Prepare(Signature{Result: I32, Args: []Type{Struct}, ArgTypes: []TypeDesc{desc}})
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	backend := plan.backend.(*ffiBackend)
	first, err := backend.acquireRecords()
	if err != nil {
		t.Fatal(err)
	}
	active := first
	defer func() {
		if active != nil {
			backend.releaseRecords(active)
		}
	}()
	second, err := backend.acquireRecords()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.releaseRecords(second)
	if first == second || first.args[0] == second.args[0] || first.out == second.out {
		t.Fatal("active calls shared native storage")
	}
	original := first.args[0]
	for i, mem := range first.roots.allocations {
		bytes := unsafe.Slice((*byte)(mem), int(first.roots.buffers[i].size))
		for j := range bytes {
			bytes[j] = 0xa5
		}
	}
	// Populate every per-call owner, including an ephemeral aggregate type and
	// a marshaling error. Releasing the context must drop them all.
	value := Int32(42)
	first.pool.pointers[&value] = nativeCopy{value: &value}
	first.pool.owners[0x1234] = &value
	first.pool.err = errors.New("failed marshaling")
	if _, err := first.pool.allocate(16); err != nil {
		t.Fatal(err)
	}
	if _, err := first.pool.build(desc); err != nil {
		t.Fatal(err)
	}
	backend.releaseRecords(first)
	active = nil
	if len(first.pool.allocations) != 0 || len(first.pool.layouts) != 0 || len(first.pool.buffers) != 0 || len(first.pool.pointers) != 0 || len(first.pool.owners) != 0 || first.pool.err != nil {
		t.Fatal("idle context retained invocation state")
	}
	reused, err := backend.acquireRecords()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.releaseRecords(reused)
	if reused != first || reused.args[0] != original {
		t.Fatal("fixed native buffers were not reused")
	}
	for i, mem := range reused.roots.allocations {
		for _, value := range unsafe.Slice((*byte)(mem), int(reused.roots.buffers[i].size)) {
			if value != 0 {
				t.Fatal("argument/result storage, including padding, was not zeroed")
			}
		}
	}
}

func TestRecordBufferCountLimitAndClose(t *testing.T) {
	plan, err := Prepare(Signature{Result: I32, Args: []Type{I32}})
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	backend := plan.backend.(*ffiBackend)
	var works []*recordBuffer
	for i := 0; i < maxIdleRecordBuffers+2; i++ {
		work, err := backend.acquireRecords()
		if err != nil {
			for _, active := range works {
				backend.releaseRecords(active)
			}
			t.Fatal(err)
		}
		works = append(works, work)
	}
	for _, work := range works {
		backend.releaseRecords(work)
	}
	if len(backend.records) != maxIdleRecordBuffers {
		t.Fatal("idle buffer count was not bounded")
	}
	for _, work := range works[maxIdleRecordBuffers:] {
		if work.call != nil || len(work.roots.allocations) != 0 {
			t.Fatal("evicted native storage was retained")
		}
	}
	if err := plan.Close(); err != nil {
		t.Fatal(err)
	}
	if len(backend.records) != 0 || backend.recordBytes != 0 {
		t.Fatal("Close retained idle buffers")
	}
	for _, work := range works {
		if work.call != nil || work.args != nil || work.out != nil || len(work.roots.allocations) != 0 {
			t.Fatal("Close did not release native call storage")
		}
	}
}

func TestRecordBufferByteLimit(t *testing.T) {
	array := TypeDesc{Type: Array, Len: 4096, Elem: &TypeDesc{Type: U64}}
	desc := TypeDesc{Type: Struct, Fields: []Field{{Name: "values", Type: array}}}
	sig := Signature{Result: Void}
	// Two contexts exceed the 1 MiB idle budget while individual values stay
	// within the existing 64 KiB marshaling limit. No native address is called.
	for i := 0; i < 17; i++ {
		sig.Args = append(sig.Args, Struct)
		sig.ArgTypes = append(sig.ArgTypes, desc)
	}
	plan, err := Prepare(sig)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	backend := plan.backend.(*ffiBackend)
	first, err := backend.acquireRecords()
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.acquireRecords()
	if err != nil {
		backend.releaseRecords(first)
		t.Fatal(err)
	}
	backend.releaseRecords(first)
	backend.releaseRecords(second)
	if len(backend.records) != 1 || backend.recordBytes != first.size || backend.recordBytes > maxIdleRecordBytes || second.call != nil {
		t.Fatal("idle native byte budget was not enforced")
	}
}

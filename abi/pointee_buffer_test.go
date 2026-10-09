//go:build libffi && cgo

package abi

import (
	"errors"
	"testing"
	"unsafe"
)

func TestPointeeBufferReuseZeroingAndBestFit(t *testing.T) {
	_, backend := scalarBufferPlan(t, 1)
	cache := &nativeValueCache{}
	pool := nativePool{abi: backend.pool.abi, valueCache: cache, pointers: make(map[*Value]nativeCopy), owners: make(map[uint64]*Value)}
	defer cache.close()
	defer pool.close()
	for _, size := range []uint64{8, 128, 64} {
		mem, err := pool.allocate(size)
		if err != nil {
			t.Fatal(err)
		}
		for i := range unsafe.Slice((*byte)(mem), int(size)) {
			unsafe.Slice((*byte)(mem), int(size))[i] = 0xa5
		}
	}
	original := append([]*nativeValueBuffer(nil), pool.values...)
	owner := Int32(42)
	pool.pointers[&owner] = nativeCopy{value: &owner}
	pool.owners[uint64(uintptr(original[0].memory))] = &owner
	pool.err = errors.New("failed marshaling")
	desc := TypeDesc{Type: Struct, Fields: []Field{{Type: TypeDesc{Type: U64}}}}
	if _, err := pool.build(desc); err != nil {
		t.Fatal(err)
	}
	pool.reset()
	if len(pool.allocations) != 0 || len(pool.layouts) != 0 || len(pool.values) != 0 || len(pool.buffers) != 0 || len(pool.pointers) != 0 || len(pool.owners) != 0 || pool.err != nil {
		t.Fatal("idle cache retained invocation owners, layouts or errors")
	}
	if cache.bytes != 200 || len(cache.idle) != 3 {
		t.Fatal("idle value accounting is incorrect")
	}
	for _, value := range original {
		for _, b := range unsafe.Slice((*byte)(value.memory), int(value.size)) {
			if b != 0 {
				t.Fatal("idle value retained native data or padding")
			}
		}
	}
	for i, size := range []uint64{96, 33, 1} {
		mem, err := pool.allocate(size)
		if err != nil {
			t.Fatal(err)
		}
		want := original[[]int{1, 2, 0}[i]]
		if mem != want.memory || pool.values[i] != want || pool.buffers[i].size != want.size {
			t.Fatal("smallest sufficient buffer or its full lifetime range was not reused")
		}
	}
	if len(cache.idle) != 0 || cache.bytes != 0 {
		t.Fatal("borrowed value remained available to another call")
	}
	pool.reset()
	cache.close()
	for _, value := range original {
		if value.memory != nil {
			t.Fatal("Close retained native pointee storage")
		}
	}
}

func TestPointeeBufferLimitsAndActiveClose(t *testing.T) {
	for _, size := range []uint64{8, 65536} {
		cache := &nativeValueCache{}
		pool := nativePool{valueCache: cache}
		var values []*nativeValueBuffer
		for i := 0; i < maxIdlePointeeBuffers+2; i++ {
			if _, err := pool.allocate(size); err != nil {
				pool.close()
				cache.close()
				t.Fatal(err)
			}
		}
		values = append(values, pool.values...)
		pool.reset()
		want := maxIdlePointeeBuffers
		if size == 65536 {
			want = maxIdlePointeeBytes / 65536
		}
		if len(cache.idle) != want || cache.bytes != uint64(want)*size {
			t.Fatal("pointee cache count/byte bounds were not enforced")
		}
		for _, value := range values[want:] {
			if value.memory != nil {
				t.Fatal("evicted pointee buffer was not freed")
			}
		}
		if _, err := pool.allocate(size); err != nil {
			cache.close()
			t.Fatal(err)
		}
		// Final cleanup also frees a borrowed value that was not returned.
		pool.close()
		cache.close()
		for _, value := range values {
			if value.memory != nil {
				t.Fatal("final cleanup retained active or idle values")
			}
		}
		if len(cache.idle) != 0 || cache.bytes != 0 || len(pool.values) != 0 {
			t.Fatal("final cleanup retained pointee cache state")
		}
	}
}

func TestRecordPointeesSharePlanByteBudget(t *testing.T) {
	plan, err := Prepare(Signature{Result: Pointer, Args: []Type{Pointer}})
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	backend := plan.backend.(*ffiBackend)
	var works []*recordBuffer
	var values []*nativeValueBuffer
	for i := 0; i < maxIdleRecordBuffers; i++ {
		work, err := backend.acquireRecords()
		if err != nil {
			for _, active := range works {
				backend.releaseRecords(active)
			}
			t.Fatal(err)
		}
		works = append(works, work)
		for j := 0; j < maxIdlePointeeBytes/65536; j++ {
			if _, err := work.pool.allocate(65536); err != nil {
				for _, active := range works {
					backend.releaseRecords(active)
				}
				t.Fatal(err)
			}
		}
		values = append(values, work.pool.values...)
	}
	for _, work := range works {
		backend.releaseRecords(work)
	}
	// Four 256 KiB pointee caches plus fixed storage exceed the shared 1 MiB.
	if len(backend.records) != 3 || backend.recordBytes > maxIdleRecordBytes || works[3].call != nil {
		t.Fatal("pointee storage bypassed the plan's native byte budget")
	}
	var total uint64
	for _, work := range backend.records {
		if work.size != work.fixedSize+work.pointees.bytes || len(work.pool.values) != 0 {
			t.Fatal("idle record size or borrowed value accounting is incorrect")
		}
		total += work.size
	}
	if backend.recordBytes != total {
		t.Fatal("plan accounting omitted retained pointee storage")
	}
	if err := plan.Close(); err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		if value.memory != nil {
			t.Fatal("context eviction/Close did not free pointee memory")
		}
	}
}

func TestPointeeReuseKeepsPaddingAndInteriorPointerChecks(t *testing.T) {
	cache := &nativeValueCache{}
	pool := nativePool{valueCache: cache, owners: make(map[uint64]*Value), pointers: make(map[*Value]nativeCopy)}
	defer cache.close()
	defer pool.close()
	mem, err := pool.allocate(128)
	if err != nil {
		t.Fatal(err)
	}
	pool.reset()
	reused, err := pool.allocate(8)
	if err != nil {
		t.Fatal(err)
	}
	if reused != mem || pool.buffers[0].size != 128 {
		t.Fatal("smaller shape did not retain the full capacity guard")
	}
	// An address in unused capacity must not escape as caller-owned memory.
	var pointer uintptr = uintptr(mem) + 64
	if got := pool.read(TypeDesc{Type: Pointer}, nil, unsafe.Pointer(&pointer), false); got.Bits != 0 || pool.err == nil {
		t.Fatal("returned pointer into reused padding escaped the call")
	}
}

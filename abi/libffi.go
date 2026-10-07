//go:build libffi && cgo

package abi

/*
#cgo pkg-config: libffi
#include <ffi.h>
#include <stdint.h>
#include <string.h>
#include <stdlib.h>
typedef union { ffi_arg word; int32_t i32; uint32_t u32; int64_t i64;
    uint64_t u64; int8_t i8; uint8_t u8; int16_t i16; uint16_t u16; float f32; double f64; void *ptr; } dylib_scalar;
static ffi_type *dylib_ffi_type(uint8_t t) {
    switch(t) {
    case 0: return &ffi_type_void; case 1: return &ffi_type_sint32;
    case 2: return &ffi_type_uint32; case 3: return &ffi_type_sint64;
    case 4: return &ffi_type_uint64; case 5: return &ffi_type_float;
    case 6: return &ffi_type_double; case 7: return &ffi_type_pointer;
    case 8: return &ffi_type_sint8; case 9: return &ffi_type_uint8;
    case 10: return &ffi_type_sint16; case 11: return &ffi_type_uint16;
    case 12: return &ffi_type_uint8;
    default: return NULL;
    }
}
static int dylib_ffi_call(uintptr_t address, uint8_t result, unsigned n,
    const uint8_t *types, const uint64_t *bits, uint64_t *out) {
    if (n > 32) return -1;
    ffi_type *atypes[32]; dylib_scalar storage[32]; void *args[32];
    for (unsigned i=0;i<n;i++) {
        atypes[i]=dylib_ffi_type(types[i]); args[i]=&storage[i];
        if (!atypes[i] || types[i]==0) return -1;
        switch(types[i]) {
        case 1: storage[i].i32=(int32_t)bits[i]; break;
        case 2: storage[i].u32=(uint32_t)bits[i]; break;
        case 3: storage[i].i64=(int64_t)bits[i]; break;
        case 4: storage[i].u64=bits[i]; break;
        case 5: { uint32_t b=(uint32_t)bits[i]; memcpy(&storage[i].f32,&b,4); break; }
        case 6: memcpy(&storage[i].f64,&bits[i],8); break;
        case 7: storage[i].ptr=(void *)(uintptr_t)bits[i]; break;
        case 8: storage[i].i8=(int8_t)bits[i]; break;
        case 9: case 12: storage[i].u8=(uint8_t)bits[i]; break;
        case 10: storage[i].i16=(int16_t)bits[i]; break;
        case 11: storage[i].u16=(uint16_t)bits[i]; break;
        }
    }
    ffi_type *rtype=dylib_ffi_type(result); if (!rtype) return -1;
    ffi_cif cif; int rc=ffi_prep_cif(&cif,FFI_DEFAULT_ABI,n,rtype,atypes);
    if (rc!=FFI_OK) return rc;
    dylib_scalar ret; memset(&ret,0,sizeof(ret));
    ffi_call(&cif,FFI_FN(address),&ret,args);
    switch(result) {
    case 0: *out=0; break;
    case 1: case 2: *out=(uint32_t)ret.word; break;
    case 3: case 4: *out=ret.u64; break;
    case 5: { uint32_t b; memcpy(&b,&ret.f32,4); *out=b; break; }
    case 6: memcpy(out,&ret.f64,8); break;
    case 7: *out=(uintptr_t)ret.ptr; break;
    case 8: case 9: case 12: *out=(uint8_t)ret.word; break;
    case 10: case 11: *out=(uint16_t)ret.word; break;
    }
    return 0;
}
typedef struct {
    ffi_type type;
    ffi_type *elements[33];
    size_t offsets[32];
} dylib_struct_type;
static dylib_struct_type *dylib_struct_new(void) {
    dylib_struct_type *t=(dylib_struct_type *)calloc(1,sizeof(*t));
    if (t) { t->type.type=FFI_TYPE_STRUCT; t->type.elements=t->elements; }
    return t;
}
static void dylib_struct_field(dylib_struct_type *t, unsigned i, ffi_type *f) { t->elements[i]=f; }
static int dylib_struct_layout(dylib_struct_type *t) { return ffi_get_struct_offsets(FFI_DEFAULT_ABI,&t->type,t->offsets); }
static ffi_type *dylib_struct_ffi(dylib_struct_type *t) { return &t->type; }
static size_t dylib_struct_offset(dylib_struct_type *t, unsigned i) { return t->offsets[i]; }
static size_t dylib_type_size(ffi_type *t) { return t->size; }
static void *dylib_field_address(void *p, size_t offset) { return (char *)p+offset; }
static void dylib_store(void *p, uint8_t t, uint64_t bits) {
    switch(t) {
    case 1: { int32_t v=(int32_t)bits; memcpy(p,&v,4); break; }
    case 2: { uint32_t v=(uint32_t)bits; memcpy(p,&v,4); break; }
    case 3: case 4: case 6: memcpy(p,&bits,8); break;
    case 5: { uint32_t v=(uint32_t)bits; memcpy(p,&v,4); break; }
    case 7: { void *v=(void *)(uintptr_t)bits; memcpy(p,&v,sizeof(v)); break; }
    case 8: case 9: case 12: { uint8_t v=(uint8_t)bits; memcpy(p,&v,1); break; }
    case 10: case 11: { uint16_t v=(uint16_t)bits; memcpy(p,&v,2); break; }
    }
}
static uint64_t dylib_read(void *p, uint8_t t, int is_result) {
    uint64_t bits=0;
    if (is_result && ((t>=1 && t<=2) || (t>=8 && t<=12))) {
        ffi_arg word; memcpy(&word,p,sizeof(word));
        if (t==8 || t==9 || t==12) return (uint8_t)word;
        if (t==10 || t==11) return (uint16_t)word;
        return (uint32_t)word;
    }
    switch(t) {
    case 1: case 2: case 5: { uint32_t v; memcpy(&v,p,4); bits=v; break; }
    case 3: case 4: case 6: memcpy(&bits,p,8); break;
    case 7: { void *v; memcpy(&v,p,sizeof(v)); bits=(uintptr_t)v; break; }
    case 8: case 9: case 12: { uint8_t v; memcpy(&v,p,1); bits=v; break; }
    case 10: case 11: { uint16_t v; memcpy(&v,p,2); bits=v; break; }
    }
    return bits;
}
typedef struct {
    unsigned n;
    ffi_type *result;
    ffi_type *types[32];
    void *args[32];
} dylib_record_call;
static dylib_record_call *dylib_record_new(ffi_type *result, unsigned n) {
    dylib_record_call *c=(dylib_record_call *)calloc(1,sizeof(*c));
    if (c) { c->n=n; c->result=result; }
    return c;
}
static void dylib_record_arg(dylib_record_call *c, unsigned i, ffi_type *t, void *p) { c->types[i]=t; c->args[i]=p; }
static int dylib_record_execute(dylib_record_call *c, uintptr_t address, void *out) {
    ffi_cif cif;
    int rc=ffi_prep_cif(&cif,FFI_DEFAULT_ABI,c->n,c->result,c->types);
    if (rc!=FFI_OK) return rc;
    ffi_call(&cif,FFI_FN(address),out,c->args);
    return 0;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func Available() bool { return true }
func invoke(address uintptr, s Signature, args []Value) (Value, error) {
	records := s.Result == Struct
	for _, v := range args {
		records = records || v.Type == Struct || v.Pointee != nil
	}
	if records {
		return invokeRecords(address, s, args)
	}
	var types [32]C.uint8_t
	var bits [32]C.uint64_t
	for i, v := range args {
		types[i] = C.uint8_t(v.Type)
		bits[i] = C.uint64_t(v.Bits)
	}
	var out C.uint64_t
	rc := C.dylib_ffi_call(C.uintptr_t(address), C.uint8_t(s.Result), C.uint(len(args)), &types[0], &bits[0], &out)
	if rc != 0 {
		return Value{}, fmt.Errorf("ffi call preparation failed: %d", rc)
	}
	return Value{Type: s.Result, Bits: uint64(out)}, nil
}

type nativeType struct {
	ffi     *C.ffi_type
	fields  []*nativeType
	offsets []C.size_t
}
type nativeCopy struct {
	value  *Value
	desc   TypeDesc
	native *nativeType
	memory unsafe.Pointer
}
type nativePool struct {
	allocations []unsafe.Pointer
	buffers     []nativeBuffer
	pointers    map[*Value]nativeCopy
	owners      map[uint64]*Value
	err         error
}
type nativeBuffer struct{ base, size uint64 }

func (p *nativePool) close() {
	for i := len(p.allocations) - 1; i >= 0; i-- {
		C.free(p.allocations[i])
	}
}
func (p *nativePool) allocate(size uint64) (unsafe.Pointer, error) {
	if size > 65536 {
		return nil, fmt.Errorf("native value exceeds 64 KiB")
	}
	if size < 8 {
		size = 8
	}
	mem := C.calloc(1, C.size_t(size))
	if mem == nil {
		return nil, fmt.Errorf("native allocation failed")
	}
	p.allocations = append(p.allocations, mem)
	p.buffers = append(p.buffers, nativeBuffer{uint64(uintptr(mem)), size})
	return mem, nil
}
func (p *nativePool) build(d TypeDesc) (*nativeType, error) {
	if d.Type != Struct {
		return &nativeType{ffi: C.dylib_ffi_type(C.uint8_t(d.Type))}, nil
	}
	t := C.dylib_struct_new()
	if t == nil {
		return nil, fmt.Errorf("native type allocation failed")
	}
	p.allocations = append(p.allocations, unsafe.Pointer(t))
	n := &nativeType{ffi: C.dylib_struct_ffi(t)}
	for i, f := range d.Fields {
		child, err := p.build(f.Type)
		if err != nil {
			return nil, err
		}
		n.fields = append(n.fields, child)
		C.dylib_struct_field(t, C.uint(i), child.ffi)
	}
	if rc := C.dylib_struct_layout(t); rc != 0 {
		return nil, fmt.Errorf("ffi struct layout failed: %d", rc)
	}
	if uint64(C.dylib_type_size(n.ffi)) > 65536 {
		return nil, fmt.Errorf("native struct exceeds 64 KiB")
	}
	for i := range d.Fields {
		n.offsets = append(n.offsets, C.dylib_struct_offset(t, C.uint(i)))
	}
	return n, nil
}
func (p *nativePool) write(v Value, d TypeDesc, n *nativeType, mem unsafe.Pointer) error {
	if d.Type == Struct {
		for i, f := range d.Fields {
			if err := p.write(v.Aggregate.Fields[i], f.Type, n.fields[i], C.dylib_field_address(mem, n.offsets[i])); err != nil {
				return err
			}
		}
		return nil
	}
	bits := v.Bits
	if v.Pointee != nil {
		copy, ok := p.pointers[v.Pointee]
		if !ok {
			e := v.Pointee.Description()
			if d.Elem != nil {
				e = *d.Elem
			}
			t, err := p.build(e)
			if err != nil {
				return err
			}
			data, err := p.allocate(uint64(C.dylib_type_size(t.ffi)))
			if err != nil {
				return err
			}
			copy = nativeCopy{value: v.Pointee, desc: e, native: t, memory: data}
			p.pointers[v.Pointee] = copy
			p.owners[uint64(uintptr(data))] = v.Pointee
			if err := p.write(*v.Pointee, e, t, data); err != nil {
				return err
			}
		}
		bits = uint64(uintptr(copy.memory))
	}
	C.dylib_store(mem, C.uint8_t(d.Type), C.uint64_t(bits))
	return nil
}
func (p *nativePool) read(d TypeDesc, n *nativeType, mem unsafe.Pointer, result bool) Value {
	v := Value{Type: d.Type}
	if d.Type == Struct {
		v.Aggregate = &Aggregate{Type: d.Clone()}
		for i, f := range d.Fields {
			v.Aggregate.Fields = append(v.Aggregate.Fields, p.read(f.Type, n.fields[i], C.dylib_field_address(mem, n.offsets[i]), false))
		}
	} else {
		isResult := C.int(0)
		if result {
			isResult = 1
		}
		v.Bits = uint64(C.dylib_read(mem, C.uint8_t(d.Type), isResult))
		if d.Type == Pointer {
			if owner := p.owners[v.Bits]; owner != nil {
				if d.Elem != nil && !sameLayout(*d.Elem, p.pointers[owner].desc) {
					p.err = fmt.Errorf("returned temporary pointer has an incompatible pointee type")
				}
				v.Bits = 0
				v.Pointee = owner
			} else {
				for _, b := range p.buffers {
					if v.Bits >= b.base && v.Bits-b.base < b.size {
						p.err = fmt.Errorf("returned interior pointer into temporary native storage cannot outlive the call")
						v.Bits = 0
					}
				}
			}
		}
	}
	return v
}
func invokeRecords(address uintptr, s Signature, args []Value) (Value, error) {
	pool := nativePool{pointers: make(map[*Value]nativeCopy), owners: make(map[uint64]*Value)}
	defer pool.close()
	resultDesc := s.ReturnType()
	resultType, err := pool.build(resultDesc)
	if err != nil {
		return Value{}, err
	}
	call := C.dylib_record_new(resultType.ffi, C.uint(len(args)))
	if call == nil {
		return Value{}, fmt.Errorf("native call allocation failed")
	}
	defer C.free(unsafe.Pointer(call))
	for i, v := range args {
		d := s.ArgumentType(i)
		t, err := pool.build(d)
		if err != nil {
			return Value{}, err
		}
		mem, err := pool.allocate(uint64(C.dylib_type_size(t.ffi)))
		if err != nil {
			return Value{}, err
		}
		if err = pool.write(v, d, t, mem); err != nil {
			return Value{}, err
		}
		C.dylib_record_arg(call, C.uint(i), t.ffi, mem)
	}
	out, err := pool.allocate(uint64(C.dylib_type_size(resultType.ffi)))
	if err != nil {
		return Value{}, err
	}
	if rc := C.dylib_record_execute(call, C.uintptr_t(address), out); rc != 0 {
		return Value{}, fmt.Errorf("ffi call preparation failed: %d", rc)
	}
	for _, copy := range pool.pointers {
		*copy.value = pool.read(copy.desc, copy.native, copy.memory, false)
	}
	result := pool.read(resultDesc, resultType, out, true)
	if pool.err != nil {
		return Value{}, pool.err
	}
	return result, nil
}

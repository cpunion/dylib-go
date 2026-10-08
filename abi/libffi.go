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
typedef struct { ffi_cif cif; ffi_type *types[32]; } dylib_call_plan;
static int dylib_abi(uint8_t convention) {
    if (convention <= 1) return FFI_DEFAULT_ABI;
#if defined(_WIN32) && (defined(__i386__) || defined(_M_IX86))
    if (convention == 2) return FFI_STDCALL;
    if (convention == 3) return FFI_FASTCALL;
#endif
    return -1;
}
static dylib_call_plan *dylib_plan_new(void) { return (dylib_call_plan *)calloc(1,sizeof(dylib_call_plan)); }
static void dylib_plan_arg(dylib_call_plan *p, unsigned i, ffi_type *type) { p->types[i]=type; }
static int dylib_plan_prepare(dylib_call_plan *p, int abi, unsigned n, unsigned fixed, int variadic, ffi_type *result) {
    if (variadic) return ffi_prep_cif_var(&p->cif,(ffi_abi)abi,fixed,n,result,p->types);
    return ffi_prep_cif(&p->cif,(ffi_abi)abi,n,result,p->types);
}
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
static int dylib_ffi_call(dylib_call_plan *plan, uintptr_t address, uint8_t result, unsigned n,
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
    dylib_scalar ret; memset(&ret,0,sizeof(ret));
    ffi_call(&plan->cif,FFI_FN(address),&ret,args);
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
    size_t *offsets;
    ffi_type *elements[];
} dylib_struct_type;
static dylib_struct_type *dylib_struct_new(unsigned n) {
    dylib_struct_type *t=(dylib_struct_type *)calloc(1,sizeof(*t)+(n+1)*sizeof(ffi_type *)+n*sizeof(size_t));
    if (t) {
        t->type.type=FFI_TYPE_STRUCT; t->type.elements=t->elements;
        t->offsets=(size_t *)(t->elements+n+1);
    }
    return t;
}
static void dylib_struct_field(dylib_struct_type *t, unsigned i, ffi_type *f) { t->elements[i]=f; }
static int dylib_struct_layout(dylib_struct_type *t, int abi) { return ffi_get_struct_offsets((ffi_abi)abi,&t->type,t->offsets); }
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
static void dylib_record_execute(dylib_call_plan *plan, dylib_record_call *c, uintptr_t address, void *out) {
    ffi_call(&plan->cif,FFI_FN(address),out,c->args);
}

extern void dylibgo_dispatch_callback(uintptr_t handle, void *result, void *arguments);
typedef struct {
    void *writable;
    void *code;
    uintptr_t handle;
} dylib_callback;
static int dylib_register_result(ffi_type *type) {
    switch (type->type) {
    case FFI_TYPE_INT: case FFI_TYPE_SINT8: case FFI_TYPE_UINT8:
    case FFI_TYPE_SINT16: case FFI_TYPE_UINT16:
    case FFI_TYPE_SINT32: case FFI_TYPE_UINT32: return 1;
    }
    return 0;
}
static void dylib_closure_dispatch(ffi_cif *cif, void *out, void **args, void *user) {
    dylib_callback *callback = (dylib_callback *)user;
    if (cif->rtype->type != FFI_TYPE_VOID) {
        size_t size = cif->rtype->size;
        if (dylib_register_result(cif->rtype) && size < sizeof(ffi_arg)) size = sizeof(ffi_arg);
        memset(out, 0, size);
    }
    dylibgo_dispatch_callback(callback->handle, out, args);
}
static dylib_callback *dylib_callback_new(dylib_call_plan *plan, uintptr_t handle, int *status) {
    *status = -1;
#if FFI_CLOSURES
    dylib_callback *callback = (dylib_callback *)calloc(1,sizeof(*callback));
    if (!callback) return NULL;
    callback->writable = (ffi_closure *)ffi_closure_alloc(sizeof(ffi_closure), &callback->code);
    if (!callback->writable) { free(callback); return NULL; }
    callback->handle = handle;
    *status = ffi_prep_closure_loc(callback->writable, &plan->cif,
                                  dylib_closure_dispatch, callback, callback->code);
    if (*status != FFI_OK) { ffi_closure_free(callback->writable); free(callback); return NULL; }
    return callback;
#else
    return NULL;
#endif
}
static uintptr_t dylib_callback_code(dylib_callback *callback) { return (uintptr_t)callback->code; }
static void dylib_callback_free(dylib_callback *callback) {
#if FFI_CLOSURES
    ffi_closure_free(callback->writable);
#endif
    free(callback);
}
static void dylib_callback_store(void *out, uint8_t type, uint64_t bits) {
    switch (type) {
    case 1: { ffi_sarg v=(int32_t)bits; memcpy(out,&v,sizeof(v)); return; }
    case 8: { ffi_sarg v=(int8_t)bits; memcpy(out,&v,sizeof(v)); return; }
    case 10: { ffi_sarg v=(int16_t)bits; memcpy(out,&v,sizeof(v)); return; }
    case 2: { ffi_arg v=(uint32_t)bits; memcpy(out,&v,sizeof(v)); return; }
    case 9: case 12: { ffi_arg v=(uint8_t)bits; memcpy(out,&v,sizeof(v)); return; }
    case 11: { ffi_arg v=(uint16_t)bits; memcpy(out,&v,sizeof(v)); return; }
    }
    dylib_store(out,type,bits);
}
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"unsafe"
)

func Available() bool { return true }

type ffiBackend struct {
	plan   *C.dylib_call_plan
	pool   nativePool
	result *nativeType
	args   []*nativeType
}

func prepare(s Signature) (callBackend, error) {
	convention := C.dylib_abi(C.uint8_t(s.Convention))
	if convention < 0 {
		return nil, fmt.Errorf("calling convention %d is unavailable on this host", s.Convention)
	}
	b := &ffiBackend{pool: nativePool{abi: convention}, plan: C.dylib_plan_new()}
	if b.plan == nil {
		return nil, fmt.Errorf("native call plan allocation failed")
	}
	ok := false
	defer func() {
		if !ok {
			b.close()
		}
	}()
	var err error
	b.result, err = b.pool.build(s.ReturnType())
	if err != nil {
		return nil, err
	}
	for i := range s.Args {
		t, err := b.pool.build(s.ArgumentType(i))
		if err != nil {
			return nil, err
		}
		b.args = append(b.args, t)
		C.dylib_plan_arg(b.plan, C.uint(i), t.ffi)
	}
	variadic := C.int(0)
	if s.Variadic {
		variadic = 1
	}
	if rc := C.dylib_plan_prepare(b.plan, convention, C.uint(len(s.Args)), C.uint(s.FixedArgs), variadic, b.result.ffi); rc != 0 {
		return nil, fmt.Errorf("ffi call preparation failed: %d", rc)
	}
	ok = true
	return b, nil
}

func (b *ffiBackend) close() {
	C.free(unsafe.Pointer(b.plan))
	b.pool.close()
}

func (b *ffiBackend) invoke(address uintptr, s Signature, args []Value) (Value, error) {
	records := s.Result == Struct
	for _, v := range args {
		records = records || v.Type == Struct || v.Pointee != nil
	}
	if records {
		return b.invokeRecords(address, s, args)
	}
	var types [32]C.uint8_t
	var bits [32]C.uint64_t
	for i, v := range args {
		types[i] = C.uint8_t(v.Type)
		bits[i] = C.uint64_t(v.Bits)
	}
	var out C.uint64_t
	rc := C.dylib_ffi_call(b.plan, C.uintptr_t(address), C.uint8_t(s.Result), C.uint(len(args)), &types[0], &bits[0], &out)
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
	abi         C.int
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
	if d.Type != Struct && d.Type != Array {
		return &nativeType{ffi: C.dylib_ffi_type(C.uint8_t(d.Type))}, nil
	}
	t := C.dylib_struct_new(C.uint(d.memberCount()))
	if t == nil {
		return nil, fmt.Errorf("native type allocation failed")
	}
	p.allocations = append(p.allocations, unsafe.Pointer(t))
	n := &nativeType{ffi: C.dylib_struct_ffi(t)}
	var child *nativeType
	for i := 0; i < d.memberCount(); i++ {
		// libffi represents an array member as a struct with repeated elements.
		// Reuse its element layout; array arguments/results are rejected earlier.
		if d.Type != Array || i == 0 {
			var err error
			child, err = p.build(d.memberType(i))
			if err != nil {
				return nil, err
			}
		}
		n.fields = append(n.fields, child)
		C.dylib_struct_field(t, C.uint(i), child.ffi)
	}
	if rc := C.dylib_struct_layout(t, p.abi); rc != 0 {
		return nil, fmt.Errorf("ffi struct layout failed: %d", rc)
	}
	if uint64(C.dylib_type_size(n.ffi)) > 65536 {
		return nil, fmt.Errorf("native aggregate exceeds 64 KiB")
	}
	for i := 0; i < d.memberCount(); i++ {
		n.offsets = append(n.offsets, C.dylib_struct_offset(t, C.uint(i)))
	}
	return n, nil
}
func (p *nativePool) write(v Value, d TypeDesc, n *nativeType, mem unsafe.Pointer) error {
	if d.Type == Struct || d.Type == Array {
		for i, member := range v.Aggregate.Fields {
			if err := p.write(member, d.memberType(i), n.fields[i], C.dylib_field_address(mem, n.offsets[i])); err != nil {
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
	if d.Type == Struct || d.Type == Array {
		v.Aggregate = &Aggregate{Type: d.Clone()}
		for i := 0; i < d.memberCount(); i++ {
			v.Aggregate.Fields = append(v.Aggregate.Fields, p.read(d.memberType(i), n.fields[i], C.dylib_field_address(mem, n.offsets[i]), false))
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
func (b *ffiBackend) invokeRecords(address uintptr, s Signature, args []Value) (Value, error) {
	pool := nativePool{abi: b.pool.abi, pointers: make(map[*Value]nativeCopy), owners: make(map[uint64]*Value)}
	defer pool.close()
	resultDesc := s.ReturnType()
	resultType := b.result
	call := C.dylib_record_new(resultType.ffi, C.uint(len(args)))
	if call == nil {
		return Value{}, fmt.Errorf("native call allocation failed")
	}
	defer C.free(unsafe.Pointer(call))
	for i, v := range args {
		d := s.ArgumentType(i)
		t := b.args[i]
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
	C.dylib_record_execute(b.plan, call, C.uintptr_t(address), out)
	for _, copy := range pool.pointers {
		*copy.value = pool.read(copy.desc, copy.native, copy.memory, false)
	}
	result := pool.read(resultDesc, resultType, out, true)
	if pool.err != nil {
		return Value{}, pool.err
	}
	return result, nil
}

type ffiCallbackBackend struct {
	plan   *ffiBackend
	native *C.dylib_callback
	handle uintptr
}

func prepareCallback(state *callbackState) (callbackBackend, error) {
	backend, err := prepare(state.signature)
	if err != nil {
		return nil, err
	}
	b := &ffiCallbackBackend{plan: backend.(*ffiBackend), handle: uintptr(cgo.NewHandle(state))}
	var status C.int
	b.native = C.dylib_callback_new(b.plan.plan, C.uintptr_t(b.handle), &status)
	if b.native == nil {
		cgo.Handle(b.handle).Delete()
		b.plan.close()
		return nil, fmt.Errorf("native callback preparation failed: %d", status)
	}
	return b, nil
}

func (b *ffiCallbackBackend) address() uintptr { return uintptr(C.dylib_callback_code(b.native)) }
func (b *ffiCallbackBackend) close() {
	C.dylib_callback_free(b.native)
	cgo.Handle(b.handle).Delete()
	b.plan.close()
}
func (b *ffiCallbackBackend) dispatch(state *callbackState, result, arguments unsafe.Pointer) {
	args := make([]Value, len(state.signature.Args))
	pointers := unsafe.Slice((*unsafe.Pointer)(arguments), len(args))
	// Native argument storage and pointee addresses are borrowed for this call.
	// The type graph is immutable; each invocation owns its logical Go values.
	pool := nativePool{abi: b.plan.pool.abi}
	for i := range args {
		args[i] = pool.read(state.signature.ArgumentType(i), b.plan.args[i], pointers[i], false)
	}
	value, err := state.handler(args)
	if err == nil {
		err = value.Validate(state.signature.ReturnType())
	}
	if err == nil && temporaryCallbackResult(value) {
		err = fmt.Errorf("callback result cannot contain temporary pointees")
	}
	if err != nil {
		state.record(err)
		return
	}
	if value.Type == Void {
		return
	}
	if value.Type == Struct {
		if err := pool.write(value, state.signature.ReturnType(), b.plan.result, result); err != nil {
			state.record(err)
		}
	} else {
		C.dylib_callback_store(result, C.uint8_t(value.Type), C.uint64_t(value.Bits))
	}
}

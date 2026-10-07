//go:build libffi && cgo

package abi

/*
#cgo pkg-config: libffi
#include <ffi.h>
#include <stdint.h>
#include <string.h>
typedef union { ffi_arg word; int32_t i32; uint32_t u32; int64_t i64;
    uint64_t u64; float f32; double f64; void *ptr; } dylib_scalar;
static ffi_type *dylib_ffi_type(uint8_t t) {
    switch(t) {
    case 0: return &ffi_type_void; case 1: return &ffi_type_sint32;
    case 2: return &ffi_type_uint32; case 3: return &ffi_type_sint64;
    case 4: return &ffi_type_uint64; case 5: return &ffi_type_float;
    case 6: return &ffi_type_double; case 7: return &ffi_type_pointer;
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
    }
    return 0;
}
*/
import "C"

import "fmt"

func Available() bool { return true }
func invoke(address uintptr, s Signature, args []Value) (uint64, error) {
	var types [32]C.uint8_t
	var bits [32]C.uint64_t
	for i, v := range args {
		types[i] = C.uint8_t(v.Type)
		bits[i] = C.uint64_t(v.Bits)
	}
	var out C.uint64_t
	rc := C.dylib_ffi_call(C.uintptr_t(address), C.uint8_t(s.Result), C.uint(len(args)), &types[0], &bits[0], &out)
	if rc != 0 {
		return 0, fmt.Errorf("ffi call preparation failed: %d", rc)
	}
	return uint64(out), nil
}

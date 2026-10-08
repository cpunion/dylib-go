//go:build cgo && (darwin || linux || windows)

package native

/*
#include <stdint.h>
#include <stdlib.h>
#include <stdatomic.h>

typedef struct dylib_exit_entry {
    struct dylib_exit_entry *next;
    void (*function)(void *);
    void (*plain)(void);
    void *argument;
    void *dso;
} dylib_exit_entry;

typedef struct dylib_lifecycle {
    atomic_flag lock;
    dylib_exit_entry *entries;
} dylib_lifecycle;

static void dylib_exit_lock(dylib_lifecycle *ctx) {
    while (atomic_flag_test_and_set_explicit(&ctx->lock, memory_order_acquire)) {}
}
static void dylib_exit_unlock(dylib_lifecycle *ctx) {
    atomic_flag_clear_explicit(&ctx->lock, memory_order_release);
}
static dylib_lifecycle *dylib_lifecycle_new(void) {
    dylib_lifecycle *ctx = (dylib_lifecycle *)calloc(1, sizeof(*ctx));
    if (ctx) {
        atomic_flag_clear(&ctx->lock);
    }
    return ctx;
}
static int dylib_exit_add(dylib_lifecycle *ctx, void (*fn)(void *),
                          void (*plain)(void), void *arg, void *dso) {
    if (!fn && !plain) return -1;
    dylib_exit_entry *entry = (dylib_exit_entry *)malloc(sizeof(*entry));
    if (!entry) return -1;
    entry->function = fn;
    entry->plain = plain;
    entry->argument = arg;
    entry->dso = dso;
    dylib_exit_lock(ctx);
    entry->next = ctx->entries;
    ctx->entries = entry;
    dylib_exit_unlock(ctx);
    return 0;
}
static int dylib_cxa_atexit(void (*fn)(void *), void *arg, void *dso,
                           dylib_lifecycle *ctx) {
    return dylib_exit_add(ctx, fn, NULL, arg, dso);
}
static int dylib_atexit(void (*fn)(void), dylib_lifecycle *ctx) {
    return dylib_exit_add(ctx, NULL, fn, NULL, NULL);
}
static void dylib_cxa_finalize(void *dso, dylib_lifecycle *ctx) {
    for (;;) {
        dylib_exit_lock(ctx);
        dylib_exit_entry **cursor = &ctx->entries;
        while (*cursor && dso && (*cursor)->dso != dso) cursor = &(*cursor)->next;
        dylib_exit_entry *entry = *cursor;
        if (entry) *cursor = entry->next;
        dylib_exit_unlock(ctx);
        if (!entry) break;
        // Detach before invoking. Recursive finalize cannot invoke it twice;
        // new registrations are visited on the next iteration in LIFO order.
        void (*fn)(void *) = entry->function;
        void (*plain)(void) = entry->plain;
        void *arg = entry->argument;
        free(entry);
        if (plain) plain(); else fn(arg);
    }
}
static void dylib_lifecycle_free(dylib_lifecycle *ctx) {
    // Failed link attempts must not invoke native code, even during rollback.
    dylib_exit_entry *entry = ctx->entries;
    while (entry) {
        dylib_exit_entry *next = entry->next;
        free(entry);
        entry = next;
    }
    free(ctx);
}
static uintptr_t dylib_exit_address(int which) {
    switch (which) {
    case 0: return (uintptr_t)&dylib_atexit;
    case 1: return (uintptr_t)&dylib_cxa_atexit;
    case 2: return (uintptr_t)&dylib_cxa_finalize;
    }
    return 0;
}
static void dylib_call_void(uintptr_t fn) { ((void (*)(void))fn)(); }
static int dylib_call_initializer(uintptr_t fn) { return ((int (*)(void))fn)(); }
static void dylib_call_pointer(uintptr_t fn, uintptr_t arg) { ((void (*)(const void *))fn)((const void *)arg); }
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Lifecycle owns native exit registrations for a single image. The linker
// serializes its Go API. Native registrations are synchronized independently.
type Lifecycle struct{ context *C.dylib_lifecycle }

func NewLifecycle() (*Lifecycle, error) {
	p := C.dylib_lifecycle_new()
	if p == nil {
		return nil, fmt.Errorf("allocate native lifecycle context")
	}
	return &Lifecycle{context: p}, nil
}

func (l *Lifecycle) Context() uintptr { return uintptr(unsafe.Pointer(l.context)) }
func (l *Lifecycle) Helper(name string) uintptr {
	switch name {
	case "atexit":
		return uintptr(C.dylib_exit_address(0))
	case "__cxa_atexit":
		return uintptr(C.dylib_exit_address(1))
	case "__cxa_finalize":
		return uintptr(C.dylib_exit_address(2))
	}
	return 0
}
func (l *Lifecycle) Finalize() {
	if l != nil && l.context != nil {
		C.dylib_cxa_finalize(nil, l.context)
	}
}
func (l *Lifecycle) Close() {
	if l != nil && l.context != nil {
		C.dylib_lifecycle_free(l.context)
		l.context = nil
	}
}
func CallVoid(address uintptr) { C.dylib_call_void(C.uintptr_t(address)) }

// CallPointer invokes a native registration function with one native address.
func CallPointer(address, argument uintptr) {
	C.dylib_call_pointer(C.uintptr_t(address), C.uintptr_t(argument))
}

// CallInitializer invokes an int(void) C initializer using the native C ABI.
func CallInitializer(address uintptr) int32 {
	return int32(C.dylib_call_initializer(C.uintptr_t(address)))
}

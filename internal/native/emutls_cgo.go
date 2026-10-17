//go:build cgo && (darwin || linux || windows)

package native

/*
#cgo linux LDFLAGS: -pthread
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <stdatomic.h>
#if defined(__i386__)
// Raw MSVC i386 callers and Windows callbacks only promise four-byte incoming
// stack alignment; the GNU-built host helper may call sixteen-byte-aligned code.
#define DYLIB_TLS_ENTRY __attribute__((force_align_arg_pointer))
#else
#define DYLIB_TLS_ENTRY
#endif
#ifdef _WIN32
#include <windows.h>
#else
#include <pthread.h>
#endif

// The compiler ABI is four pointer-sized words on the supported targets.
// Only the object word is mutable. All other fields are validated in Go.
typedef struct {
    uintptr_t size, alignment;
    void *object;
    const void *initial;
} dylib_tls_descriptor;
_Static_assert(sizeof(dylib_tls_descriptor) == 4 * sizeof(uintptr_t), "emulated TLS descriptor ABI");
typedef struct dylib_tls_control dylib_tls_control;
typedef struct dylib_tls_thread dylib_tls_thread;
typedef struct dylib_tls_value dylib_tls_value;
typedef struct {
    dylib_tls_control *controls;
    size_t live;
} dylib_tls_owner;
struct dylib_tls_control {
    dylib_tls_control *next;
    dylib_tls_owner *owner;
    dylib_tls_descriptor *descriptor;
    dylib_tls_value *values;
    uintptr_t size, alignment;
    const void *initial;
};
struct dylib_tls_thread { dylib_tls_value *values; };
struct dylib_tls_value {
    dylib_tls_value *owner_next, **owner_previous;
    dylib_tls_value *thread_next, **thread_previous;
    dylib_tls_control *control;
    void *allocation, *address;
};

// One process-lifetime native key; neither its callback nor its thread state
// points into a retired image. The same lock covers owner close and thread exit.
static atomic_flag dylib_tls_lock = ATOMIC_FLAG_INIT;
static int dylib_tls_key_ready;
#ifdef _WIN32
static DWORD dylib_tls_key;
#else
static pthread_key_t dylib_tls_key;
#endif
static void dylib_tls_enter(void) {
    while (atomic_flag_test_and_set_explicit(&dylib_tls_lock, memory_order_acquire)) {}
}
static void dylib_tls_leave(void) {
    atomic_flag_clear_explicit(&dylib_tls_lock, memory_order_release);
}
static void dylib_tls_remove(dylib_tls_value *v) {
    *v->owner_previous = v->owner_next;
    if (v->owner_next) v->owner_next->owner_previous = v->owner_previous;
    *v->thread_previous = v->thread_next;
    if (v->thread_next) v->thread_next->thread_previous = v->thread_previous;
    --v->control->owner->live;
    free(v->allocation);
    free(v);
}
#ifdef _WIN32
static void DYLIB_TLS_ENTRY WINAPI dylib_tls_thread_exit(void *p)
#else
static void DYLIB_TLS_ENTRY dylib_tls_thread_exit(void *p)
#endif
{
    dylib_tls_thread *thread = (dylib_tls_thread *)p;
    dylib_tls_enter();
    while (thread->values) dylib_tls_remove(thread->values);
    dylib_tls_leave();
    free(thread);
}
static dylib_tls_owner *dylib_tls_new(void) {
    dylib_tls_enter();
    if (!dylib_tls_key_ready) {
#ifdef _WIN32
        dylib_tls_key = FlsAlloc(dylib_tls_thread_exit);
        dylib_tls_key_ready = dylib_tls_key != FLS_OUT_OF_INDEXES;
#else
        dylib_tls_key_ready = pthread_key_create(&dylib_tls_key, dylib_tls_thread_exit) == 0;
#endif
    }
    int ready = dylib_tls_key_ready;
    dylib_tls_leave();
    return ready ? (dylib_tls_owner *)calloc(1, sizeof(dylib_tls_owner)) : NULL;
}
static int dylib_tls_add(dylib_tls_owner *owner, uintptr_t address) {
    dylib_tls_control *c = (dylib_tls_control *)calloc(1, sizeof(*c));
    if (!c) return -1;
    c->owner = owner;
    c->descriptor = (dylib_tls_descriptor *)address;
    // Snapshot validated fields before native initialization can write data.
    c->size = c->descriptor->size;
    c->alignment = c->descriptor->alignment;
    c->initial = c->descriptor->initial;
    c->next = owner->controls;
    owner->controls = c;
    c->descriptor->object = c;
    return 0;
}
static void *DYLIB_TLS_ENTRY dylib_tls_get_address(dylib_tls_descriptor *descriptor) {
    dylib_tls_enter();
    dylib_tls_control *c = (dylib_tls_control *)descriptor->object;
    if (!c || c->descriptor != descriptor) abort();
#ifdef _WIN32
    dylib_tls_thread *thread = (dylib_tls_thread *)FlsGetValue(dylib_tls_key);
#else
    dylib_tls_thread *thread = (dylib_tls_thread *)pthread_getspecific(dylib_tls_key);
#endif
    if (!thread) {
        thread = (dylib_tls_thread *)calloc(1, sizeof(*thread));
        if (!thread) abort();
#ifdef _WIN32
        if (!FlsSetValue(dylib_tls_key, thread)) abort();
#else
        if (pthread_setspecific(dylib_tls_key, thread)) abort();
#endif
    }
    for (dylib_tls_value *v = thread->values; v; v = v->thread_next) {
        if (v->control == c) {
            void *address = v->address;
            dylib_tls_leave();
            return address;
        }
    }
    dylib_tls_value *v = (dylib_tls_value *)calloc(1, sizeof(*v));
    if (!v) abort();
    v->allocation = malloc(c->size + c->alignment - 1);
    if (!v->allocation) abort();
    v->address = (void *)(((uintptr_t)v->allocation + c->alignment - 1) & ~(c->alignment - 1));
    if (c->initial) memcpy(v->address, c->initial, c->size);
    else memset(v->address, 0, c->size);
    v->control = c;
    v->owner_next = c->values;
    v->owner_previous = &c->values;
    if (c->values) c->values->owner_previous = &v->owner_next;
    c->values = v;
    v->thread_next = thread->values;
    v->thread_previous = &thread->values;
    if (thread->values) thread->values->thread_previous = &v->thread_next;
    thread->values = v;
    ++c->owner->live;
    void *address = v->address;
    dylib_tls_leave();
    return address;
}
static uintptr_t dylib_tls_helper(void) { return (uintptr_t)&dylib_tls_get_address; }
static size_t dylib_tls_live(dylib_tls_owner *owner) {
    dylib_tls_enter();
    size_t live = owner->live;
    dylib_tls_leave();
    return live;
}
static void dylib_tls_free(dylib_tls_owner *owner) {
    dylib_tls_enter();
    while (owner->controls) {
        dylib_tls_control *c = owner->controls;
        while (c->values) dylib_tls_remove(c->values);
        c->descriptor->object = NULL;
        owner->controls = c->next;
        free(c);
    }
    dylib_tls_leave();
    free(owner);
}
*/
import "C"

import "fmt"

// EmulatedTLS owns native per-thread values for one raw image. Close requires
// all native users to have stopped, as enforced by the session's code leases.
type EmulatedTLS struct{ context *C.dylib_tls_owner }

func NewEmulatedTLS() (*EmulatedTLS, error) {
	p := C.dylib_tls_new()
	if p == nil {
		return nil, fmt.Errorf("allocate native emulated TLS owner/key")
	}
	return &EmulatedTLS{context: p}, nil
}
func EmulatedTLSHelper() uintptr { return uintptr(C.dylib_tls_helper()) }
func (t *EmulatedTLS) Add(descriptor uintptr) error {
	if C.dylib_tls_add(t.context, C.uintptr_t(descriptor)) != 0 {
		return fmt.Errorf("allocate native emulated TLS control")
	}
	return nil
}
func (t *EmulatedTLS) LiveValues() int {
	if t == nil || t.context == nil {
		return 0
	}
	return int(C.dylib_tls_live(t.context))
}
func (t *EmulatedTLS) Close() {
	if t != nil && t.context != nil {
		C.dylib_tls_free(t.context)
		t.context = nil
	}
}

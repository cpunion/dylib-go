//go:build cgo && darwin

package native

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>
#include <mach-o/loader.h>

// LLVM libunwind's documented Darwin dynamic-section lookup protocol.
typedef struct {
    uintptr_t dso_base, dwarf_section;
    size_t dwarf_section_length;
    uintptr_t compact_unwind_section;
    size_t compact_unwind_section_length;
} dylib_unwind_sections;
typedef int (*dylib_unwind_finder)(uintptr_t, dylib_unwind_sections *);
typedef int (*dylib_unwind_update)(dylib_unwind_finder);
typedef struct { uintptr_t start, end, dwarf, size, compact, compact_size; } dylib_unwind_range;
typedef struct dylib_unwind_registration {
    struct dylib_unwind_registration *next;
    size_t count;
    int detached;
    struct mach_header_64 header;
    uintptr_t image_base;
    dylib_unwind_range ranges[];
} dylib_unwind_registration;

// Only the baseline subarchitecture is supported by the raw loader. The
// immutable native header lets libunwind classify these JIT frames, including
// when installing a C++ landing-pad IP on older arm64 system unwinders.
#if defined(__aarch64__)
static const struct mach_header_64 dylib_unwind_header = {
    MH_MAGIC_64, CPU_TYPE_ARM64, CPU_SUBTYPE_ARM64_ALL, MH_OBJECT, 0, 0, 0, 0
};
#else
static const struct mach_header_64 dylib_unwind_header = {
    MH_MAGIC_64, CPU_TYPE_X86_64, CPU_SUBTYPE_X86_64_ALL, MH_OBJECT, 0, 0, 0, 0
};
#endif

static pthread_mutex_t dylib_unwind_operations = PTHREAD_MUTEX_INITIALIZER;
static pthread_rwlock_t dylib_unwind_ranges_lock = PTHREAD_RWLOCK_INITIALIZER;
static dylib_unwind_registration *dylib_unwind_registrations;
static dylib_unwind_update dylib_unwind_add, dylib_unwind_remove;

// Called entirely in native code, with no allocation or Go/runtime callback.
static int dylib_unwind_lookup(uintptr_t pc, dylib_unwind_sections *info) {
    int found = 0;
    pthread_rwlock_rdlock(&dylib_unwind_ranges_lock);
    for (dylib_unwind_registration *r = dylib_unwind_registrations; r; r = r->next) {
        size_t lo = 0, hi = r->count;
        while (lo < hi) {
            size_t mid = lo + (hi-lo)/2;
            if (r->ranges[mid].end <= pc) lo = mid+1; else hi = mid;
        }
        if (lo < r->count && r->ranges[lo].start <= pc) {
            info->dso_base = r->image_base;
            info->dwarf_section = r->ranges[lo].dwarf;
            info->dwarf_section_length = r->ranges[lo].size;
            info->compact_unwind_section = r->ranges[lo].compact;
            info->compact_unwind_section_length = r->ranges[lo].compact_size;
            found = 1;
            break;
        }
    }
    pthread_rwlock_unlock(&dylib_unwind_ranges_lock);
    return found;
}

static uintptr_t dylib_unwind_register(uintptr_t add, uintptr_t remove, uintptr_t header,
                                     const uintptr_t *ranges, size_t count, int *status) {
    *status = -6547;
    if (!count || count > (SIZE_MAX-sizeof(dylib_unwind_registration))/sizeof(dylib_unwind_range)) return 0;
    dylib_unwind_registration *r = malloc(sizeof(*r)+count*sizeof(r->ranges[0]));
    if (!r) { *status = -6541; return 0; }
    r->count = count;
    r->detached = 0;
    r->header = dylib_unwind_header;
    r->image_base = header ? header : (uintptr_t)&r->header;
    memcpy(r->ranges, ranges, count*sizeof(r->ranges[0]));
    pthread_mutex_lock(&dylib_unwind_operations);
    if (dylib_unwind_add && ((uintptr_t)dylib_unwind_add != add || (uintptr_t)dylib_unwind_remove != remove)) {
        *status = -6547;
    } else if (!dylib_unwind_add) {
        *status = ((dylib_unwind_update)add)(dylib_unwind_lookup);
        if (!*status) {
            dylib_unwind_add = (dylib_unwind_update)add;
            dylib_unwind_remove = (dylib_unwind_update)remove;
        }
    } else *status = 0;
    if (!*status) {
        pthread_rwlock_wrlock(&dylib_unwind_ranges_lock);
        r->next = dylib_unwind_registrations;
        dylib_unwind_registrations = r;
        pthread_rwlock_unlock(&dylib_unwind_ranges_lock);
    }
    pthread_mutex_unlock(&dylib_unwind_operations);
    if (*status) { free(r); return 0; }
    return (uintptr_t)r;
}

static int dylib_unwind_unregister(uintptr_t handle, uintptr_t clear_group) {
    dylib_unwind_registration *r = (dylib_unwind_registration *)handle;
    pthread_mutex_lock(&dylib_unwind_operations);
    if (!r->detached) {
        pthread_rwlock_wrlock(&dylib_unwind_ranges_lock);
        dylib_unwind_registration **p = &dylib_unwind_registrations;
        while (*p && *p != r) p = &(*p)->next;
        if (!*p) {
            pthread_rwlock_unlock(&dylib_unwind_ranges_lock);
            pthread_mutex_unlock(&dylib_unwind_operations);
            return -6547;
        }
        *p = r->next;
        r->detached = 1;
        pthread_rwlock_unlock(&dylib_unwind_ranges_lock);
    }
    // Never hold our range lock while libunwind takes its callback lock:
    // callback execution acquires those locks in the opposite order.
    int status = 0;
    if (!dylib_unwind_registrations && dylib_unwind_remove) {
        status = dylib_unwind_remove(dylib_unwind_lookup);
        if (!status) dylib_unwind_add = dylib_unwind_remove = NULL;
    }
    pthread_mutex_unlock(&dylib_unwind_operations);
    if (!status) {
        // Darwin's DWARF cache groups lookup-discovered entries by dso_base.
        // Its deregistration entry removes that key without reading FDE bytes.
        // Use a distinct native header per owner so retirement cannot clear a
        // different session's cache. No range lock is held during invalidation.
        ((void (*)(const void *))clear_group)((const void *)r->image_base);
        free(r);
    } // Retain the detached handle on a removal error.
    return status;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// RegisterMachOUnwind copies sorted [start,end,DWARF,size,compact,size] ranges.
// Header and section addresses must refer to retained native memory, never Go
// heap storage. A zero header uses an embedded native header for DWARF-only data.
func RegisterMachOUnwind(add, remove, header uintptr, ranges []uintptr) (uintptr, error) {
	if add == 0 || remove == 0 || len(ranges) == 0 || len(ranges)%6 != 0 {
		return 0, fmt.Errorf("invalid dynamic Mach-O unwind registration")
	}
	var status C.int
	handle := uintptr(C.dylib_unwind_register(C.uintptr_t(add), C.uintptr_t(remove), C.uintptr_t(header), (*C.uintptr_t)(unsafe.Pointer(&ranges[0])), C.size_t(len(ranges)/6), &status))
	if handle == 0 {
		return 0, fmt.Errorf("dynamic Mach-O unwind registration: %d", int(status))
	}
	return handle, nil
}

func UnregisterMachODWARF(handle, clearGroup uintptr) error {
	if handle == 0 {
		return nil
	}
	if clearGroup == 0 {
		return fmt.Errorf("dynamic Mach-O unwind removal requires cache invalidation")
	}
	if status := C.dylib_unwind_unregister(C.uintptr_t(handle), C.uintptr_t(clearGroup)); status != 0 {
		return fmt.Errorf("dynamic Mach-O unwind removal: %d", int(status))
	}
	return nil
}

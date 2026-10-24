#include "elf_arm_tlsld_provider.c"
static int live, destroyed;
struct Owner {
    Owner() { __atomic_fetch_add(&live, 1, __ATOMIC_RELAXED); }
    ~Owner() {
        __atomic_fetch_sub(&live, 1, __ATOMIC_RELAXED);
        __atomic_fetch_add(&destroyed, 1, __ATOMIC_RELAXED);
    }
    int read() const { return tls_shared; }
};
static thread_local Owner owner;
extern "C" int tls_read(void) { return owner.read(); }
struct Check {
    ~Check() {
        // Four joined C threads exit first; the current thread exits on Close.
        if (live != 0 || destroyed != 5) __builtin_trap();
    }
};
static Check check;

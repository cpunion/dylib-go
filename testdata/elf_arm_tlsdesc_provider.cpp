extern "C" {
thread_local int tls_shared = 7;
}
static int live, destroyed;
struct Owner {
    int marker;
    Owner() : marker(42) { __atomic_fetch_add(&live, 1, __ATOMIC_RELAXED); }
    ~Owner() {
        if (marker != 42) __builtin_trap();
        __atomic_fetch_sub(&live, 1, __ATOMIC_RELAXED);
        __atomic_fetch_add(&destroyed, 1, __ATOMIC_RELAXED);
    }
    int read() const { return marker == 42 ? tls_shared : -1; }
};
static thread_local Owner owner;
extern "C" int tls_read() { return owner.read(); }
struct Check {
    ~Check() { if (live != 0 || destroyed != 5) __builtin_trap(); }
};
static Check check;

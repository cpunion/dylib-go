static int live, destroyed;
static int initial() { return 2; }
struct Value {
    int value;
    Value() : value(initial()) { ++live; }
    ~Value() { --live; ++destroyed; }
};
static thread_local Value state;
struct Check {
    ~Check() {
        if (live != 0 || destroyed != 1) __builtin_trap();
    }
};
static Check check;
extern "C" int cli_cxx_tls_add(int a, int b) {
    state.value += a + b;
    return state.value;
}

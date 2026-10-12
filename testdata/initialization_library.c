// The test supplies a leased native callback address. No Go memory is retained.
typedef void (*event_hook)(int);
static void notify(int event) {
    ((event_hook)(__UINTPTR_TYPE__)HOOK_ADDRESS)(event);
}
__attribute__((constructor)) static void start(void) { notify(3); }
__attribute__((destructor)) static void stop(void) { notify(5); }
#ifdef _WIN32
__declspec(dllexport)
#endif
int initialization_root(int a, int b) { return a+b; }

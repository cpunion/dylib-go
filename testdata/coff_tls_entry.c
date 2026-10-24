#if defined(__i386__)
__attribute__((stdcall))
#endif
int DllMain(void *module, unsigned reason, void *reserved) {
    (void)module; (void)reason; (void)reserved;
    return 1;
}

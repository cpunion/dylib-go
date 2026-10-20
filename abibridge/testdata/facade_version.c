#include <stdint.h>
#include <stddef.h>
uint32_t dylib_abibridge_version(void) { return VERSION; }
void *dylib_abibridge_prepare(const unsigned char *signature, uint32_t size, char *message, uint32_t capacity) {
    (void)signature; (void)size; (void)message; (void)capacity; return 0;
}
int32_t dylib_abibridge_invoke(void *handle, const void *function,
    const unsigned char *arguments, uint32_t size, unsigned char *result, uint32_t result_size,
    char *message, uint32_t capacity) {
    (void)handle; (void)function; (void)arguments; (void)size; (void)result; (void)result_size; (void)message; (void)capacity; return 1;
}
void dylib_abibridge_release(void *handle) { (void)handle; }

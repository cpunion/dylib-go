#include <stdint.h>
#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT
#endif
typedef struct { int32_t a,b; } RegistrationPair;
typedef int32_t (*RegistrationCallback)(int32_t,int32_t);
static const RegistrationPair *saved_pair;
static RegistrationCallback saved_callback;

EXPORT void registration_store(const RegistrationPair *pair, RegistrationCallback callback) {
    saved_pair=pair;
    saved_callback=callback;
}
EXPORT int32_t registration_call(void) {
    return saved_pair && saved_callback ? saved_callback(saved_pair->a,saved_pair->b) : 0;
}
EXPORT int32_t registration_clear(void) {
    int32_t result=registration_call();
    saved_callback=0;
    saved_pair=0;
    return result;
}

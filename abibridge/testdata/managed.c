#include <stdint.h>
#include <stddef.h>
const void *fixture_null_metadata(void) { return 0; }
// Valid backing memory with deliberately unsupported witness layout fields.
// No witness function from these malformed descriptions can execute.
struct witnesses { const void *functions[8]; size_t size, stride; uint32_t flags, extra; };
#define BAD(NAME,SIZE,STRIDE,FLAGS) \
    static const struct witnesses NAME##_table = {{0},SIZE,STRIDE,FLAGS,0}; \
    static const void *const NAME##_metadata[] = {&NAME##_table,0}; \
    const void *fixture_bad_##NAME(void) { return NAME##_metadata+1; }
BAD(stride,8,0,7)
BAD(extent,16,8,7)
BAD(budget,8,1048584,7)
BAD(alignment,8,8,2)
BAD(multiple,8,9,7)

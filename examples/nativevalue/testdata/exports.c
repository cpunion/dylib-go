#include <stdint.h>
typedef struct { int32_t a, b; } NativePair;
static NativePair *saved_pair;
void native_value_save_pair(NativePair *pair) { saved_pair = pair; }
int32_t native_value_sum_pair(void) { return saved_pair ? saved_pair->a + saved_pair->b : -1; }

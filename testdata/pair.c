#include <stdint.h>
typedef struct { int32_t a, b; } pair;
int32_t sum_pair(pair v) { return v.a+v.b; }
pair echo_pair(pair v) { return v; }
pair *mutate_pair(pair *v) { v->a+=2; v->b-=2; return v; }

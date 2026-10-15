#include <stdint.h>
#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT __attribute__((visibility("default")))
#endif
typedef struct { float a, b; } FloatPair;
typedef struct { int64_t a, b; } IntegerPair;
typedef struct { double a; int64_t b; } MixedPair;
typedef struct { float a, rest[2]; } FloatTriple;
typedef struct { double values[4]; } DoubleQuad;
typedef struct { int32_t a, b; } Small;
typedef struct { int64_t a, b, c; } Large;
#define FP_ARGS double a,double b,double c,double d,double e,double f,double g,double h
#define FP_SUM (a+b+c+d+e+f+g+h)
EXPORT double exhausted_float(FP_ARGS, FloatPair pair, double tail) {
    return FP_SUM + pair.a + pair.b + tail;
}
EXPORT double partial_float(double a,double b,double c,double d,double e,double f,double g,FloatPair pair,double tail) {
    return a+b+c+d+e+f+g+pair.a+pair.b+tail;
}
EXPORT int64_t partial_integer(int64_t a,int64_t b,int64_t c,int64_t d,int64_t e,IntegerPair pair,int64_t tail) {
    return a+b+c+d+e+pair.a+pair.b+tail;
}
EXPORT double rollback_integer(int64_t a,int64_t b,int64_t c,int64_t d,int64_t e,int64_t f,MixedPair pair,double tail) {
    return a+b+c+d+e+f+pair.a+pair.b+tail;
}
EXPORT double rollback_float(FP_ARGS,MixedPair pair,int64_t tail) {
    return FP_SUM+pair.a+pair.b+tail;
}
EXPORT double stacked_triple(FP_ARGS,float gap,FloatTriple triple,double tail) {
    return FP_SUM+gap+triple.a+triple.rest[0]+triple.rest[1]+tail;
}
EXPORT double stacked_quad(FP_ARGS,DoubleQuad quad,double tail) {
    return FP_SUM+quad.values[0]+quad.values[1]+quad.values[2]+quad.values[3]+tail;
}
EXPORT Large hidden_result(int64_t a,int64_t b,int64_t c,int64_t d,int64_t e,Small pair,int64_t tail) {
    Large result = {a+b+c+d+e,pair.a+pair.b,tail};
    return result;
}

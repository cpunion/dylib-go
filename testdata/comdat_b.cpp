#include "comdat.hpp"
extern "C" int32_t from_b(int32_t a, int32_t b) { return comdat_increment(a) + b; }

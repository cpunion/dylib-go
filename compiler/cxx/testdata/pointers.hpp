#pragma once
#include <stdint.h>

namespace example {
using Number = int32_t;
using Character = char;
using Opaque = void;
using Slot = Number *;
using Row = Number[2];
using OtherRow = Number[3];
enum class Mode : int32_t { answer = 42 };
union Payload { int32_t integer; double decimal; };

class NativePointers {
    inline static Number values[2] = {20, 22};
    inline static Slot selected = values;
    inline static Row matrix[2] = {{20, 22}, {1, 2}};
    inline static Mode current = Mode::answer;
    inline static Payload data = {42};
public:
    static Number *numbers() { return values; }
    static const Number *readonly() { return values; }
    static Number *empty() { return nullptr; }
    static int32_t value(const Number *p) { return p ? *p : -1; }
    static int32_t add(Number *p, int32_t v) { return p ? (*p += v) : -1; }
    static Slot *slot() { return &selected; }
    static const Slot *readonly_slot() { return &selected; }
    static int32_t advance(Slot *p) { return p && *p ? *++*p : -1; }
    static int32_t read_slot(const Slot *p) { return p && *p ? **p : -1; }
    static Row *rows() { return matrix; }
    static int32_t row_sum(const Row *p) { return p ? (*p)[0] + (*p)[1] : -1; }
    static const Character *text() { return "42"; }
    static int32_t parse(const Character *p) { return p ? (p[0] - '0') * 10 + p[1] - '0' : -1; }
    static Opaque *opaque(Opaque *p) { return p; }
    static Mode *mode() { return &current; }
    static int32_t read_mode(const Mode *p) { return p ? static_cast<int32_t>(*p) : -1; }
    static Payload *payload() { return &data; }
    static int32_t read_integer(const Payload *p) { return p ? p->integer : -1; }
    static double set_decimal(Payload *p, double v) { return p ? (p->decimal = v) : -1; }
    static double read_decimal(const Payload *p) { return p ? p->decimal : -1; }
};
}

#pragma once
#include <stdint.h>

namespace example {
class Token {
    int32_t value;
public:
    explicit Token(int32_t v) : value(v) {}
    Token(const Token &) = delete;
    Token(Token &&) = delete;
    static Token make(int32_t v) { return Token(v); }
    static int32_t take(Token) { return 42; }
    int32_t read() const { return value; }
};
class StaticOnly {
    ~StaticOnly() = default;
public:
    static int32_t answer() { return 42; }
};
class UnstableValue {
public:
    ~UnstableValue() noexcept(false) { throw 99; }
    static int32_t take(UnstableValue) { return 42; }
    static UnstableValue make() { return UnstableValue(); }
};
class Counter {
    inline static int32_t count = 0;
    int32_t value;
public:
    explicit Counter(int32_t initial) : value(initial) {
        if (initial < 0) throw initial;
        ++count;
    }
    Counter(const Counter &other) : value(other.value) {
        if (value == 13) throw value;
        ++count;
    }
    virtual ~Counter() noexcept { --count; }
    int32_t add(int32_t delta) {
        if (delta < 0) throw delta;
        value += delta;
        return value;
    }
    double add(double delta) const { return value + delta; }
    void reset(int32_t next) { value = next; }
    bool positive() const noexcept { return value > 0; }
    void *echo(void *address) const { return address; }
    virtual int32_t score(int32_t delta) const { return value + delta; }
    static int32_t live() noexcept { return count; }
    static void *derived(int32_t initial);
    static int8_t signed8(int8_t x) { return x; }
    static uint8_t unsigned8(uint8_t x) { return x; }
    static int16_t signed16(int16_t x) { return x; }
    static uint16_t unsigned16(uint16_t x) { return x; }
    static uint32_t unsigned32(uint32_t x) { return x; }
    static int64_t signed64(int64_t x) { return x; }
    static uint64_t unsigned64(uint64_t x) { return x; }
    static float floating32(float x) { return x; }
    int32_t sum(const Counter &other) const { return value + other.value; }
    int32_t copied(Counter other) const {
        auto result = value + other.value;
        other.reset(0);
        return result;
    }
    int32_t mutate(Counter &other, int32_t delta) { return other.add(delta); }
    Counter clone() const { return Counter(*this); }
    Counter &self() { return *this; }
    const Counter &const_self() const { return *this; }
    static Counter *pointer(Counter *other) { return other; }
    static const Counter *const_pointer(const Counter *other) { return other; }
    int32_t mixed(int32_t delta, const Counter &left, const Counter *right) const {
        return value + delta + left.value + (right ? right->value : 0);
    }
    static int32_t nullable(const Counter *other) { return other ? other->value : 42; }
    // Reference results must use the object's address, not this overload.
    Counter *operator&() { return nullptr; }
    const Counter *operator&() const { return nullptr; }
};

using ReadOnlyCounter = const Counter;

class CallbackBox {
public:
    using Adder = int32_t (*)(int32_t, int32_t);
private:
    Adder callback;
    static int32_t add(int32_t a, int32_t b) { return a + b; }
public:
    explicit CallbackBox(Adder entry) : callback(entry) {
        if (entry && entry(0, 0) < 0) throw 99;
    }
    static Adder factory(bool fail) { if (fail) throw 99; return add; }
    static Adder empty() { return nullptr; }
    int32_t invoke(int32_t a, int32_t b) const { return apply(callback, a, b); }
    static int32_t apply(Adder entry, int32_t a, int32_t b) {
        return entry ? entry(a, b) : -1;
    }
    Adder exchange(Adder entry) { auto previous = callback; callback = entry; return previous; }
    int32_t mixed(int32_t delta, const Counter &other, Adder left, Adder right) const {
        return other.add(double(delta)) + apply(left, 5, 5) + apply(right, 5, 5);
    }
    static double wide(double (*entry)(int8_t, float, uint64_t), int8_t a, float b, uint64_t c) {
        return entry(a, b, c);
    }
    static void notify(void (*entry)(int32_t, void *), int32_t value, void *address) { entry(value, address); }
    static void *echo(void *(*entry)(void *), void *address) { return entry(address); }
    int32_t shutdown() { auto result = apply(callback, 20, 20); callback = nullptr; return result; }
};

// Counter is a secondary base: the factory supplies its correctly adjusted
// view. Go never computes a base offset or interprets a virtual/member pointer.
class Prefix {
    int64_t padding = 123;
public:
    virtual ~Prefix() = default;
};
class Derived : public Prefix, public Counter {
public:
    explicit Derived(int32_t initial) : Counter(initial) {}
    int32_t score(int32_t delta) const override { return Counter::score(delta) + 2; }
};
inline void *Counter::derived(int32_t initial) {
    return static_cast<Counter *>(new Derived(initial));
}
class VirtualLeft : public virtual Counter {
public:
    explicit VirtualLeft(int32_t initial) : Counter(initial) {}
};
class VirtualRight : public virtual Counter {
public:
    explicit VirtualRight(int32_t initial) : Counter(initial) {}
};
class Diamond : public VirtualLeft, public VirtualRight {
public:
    explicit Diamond(int32_t initial)
        : Counter(initial), VirtualLeft(initial), VirtualRight(initial) {}
    int32_t score(int32_t delta) const override { return Counter::score(delta) + 2; }
};
// These types make the compiler reject invalid manifest conversions.
class PrivateCounter : private Counter {};
class LeftCounter : public Counter {};
class RightCounter : public Counter {};
class AmbiguousCounter : public LeftCounter, public RightCounter {};
class ThrowingDestructor {
public:
    ~ThrowingDestructor() noexcept(false) { throw 99; }
};
class NullAllocation {
public:
    static void *operator new(__SIZE_TYPE__) noexcept { return nullptr; }
    static void operator delete(void *) noexcept {}
};
}

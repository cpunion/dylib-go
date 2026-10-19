#ifndef DYLIB_GO_MEMBERS_HPP
#define DYLIB_GO_MEMBERS_HPP

namespace decoy { struct Counter { void wrong_namespace(); }; }
namespace members {
struct Parent { int inherited() const; };
class Counter;
class Counter : public Parent {
    void private_default();
public:
    explicit Counter(int initial) : value(initial) {}
    Counter(const Counter&) = delete;
    ~Counter() = default;
    int add(int amount = 22) const { return value + amount; }
    int add(int amount, int extra) const { return value + amount + extra; }
    static int twice(int value) { return 2 * value; }
    int qualified() const & noexcept { return value; }
    int operator()(int amount) const { return add(amount); }
    explicit operator bool() const noexcept { return value != 0; }
    void variadic(int marker, ...);
    template<class T> T templated(T value) { return value; }
    struct Nested { void wrong_nested(); };
    friend void wrong_friend(Counter&);
protected:
    void hidden_protected();
private:
    int value;
    void hidden_private();
};
using Alias = Counter;
using ConstCounter = const Counter;
using Pointer = Counter*;
class Incomplete;
union NotClass { int value; };
using Scalar = int;
inline auto closure = [] { return 42; };
using Lambda = decltype(closure);

struct Argument {};
template<class T> struct Box {
    T value;
    int read() const { return 42; }
private:
    void hidden();
};
using Specialized = Box<Argument>;
using OtherSpecialized = Box<int>;
struct Abstract { virtual int read() const = 0; };
struct Target {
#if __SIZEOF_POINTER__ == 4
    int width32();
#else
    int width64();
#endif
#if defined(DYLIB_MEMBER_EXTRA)
    int extra();
#endif
};
}
#endif

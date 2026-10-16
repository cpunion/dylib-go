#include <cstdint>
#include <functional>
#include <memory>

namespace example {
using Function = std::function<int32_t(int32_t, int32_t)>;

inline auto capturedAdder(int32_t seed) {
    return [sum = seed](int32_t a, int32_t b) mutable -> int32_t {
        sum += a + b;
        return sum;
    };
}
using CapturedAdder = decltype(capturedAdder(0));

inline int32_t liveCallables = 0;
struct CallableState {
    int32_t sum;
    explicit CallableState(int32_t seed) : sum(seed) { ++liveCallables; }
    ~CallableState() noexcept { --liveCallables; }
};
inline auto moveOnlyAdder(int32_t seed) {
    return [state = std::make_unique<CallableState>(seed)](int32_t a, int32_t b) mutable -> int32_t {
        state->sum += a + b;
        return state->sum;
    };
}
using MoveOnlyAdder = decltype(moveOnlyAdder(0));

class CallableFactory {
    static int32_t add(int32_t a, int32_t b) { return a + b; }
public:
    static Function function(int32_t seed) { return capturedAdder(seed); }
    static CapturedAdder captured(int32_t seed) { return capturedAdder(seed); }
    static MoveOnlyAdder move_only(int32_t seed) { return moveOnlyAdder(seed); }
    static auto entry() -> int32_t (*)(int32_t, int32_t) { return &add; }
    static int32_t live() { return liveCallables; }
};
}

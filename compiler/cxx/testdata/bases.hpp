#pragma once
#include "counter.hpp"

// These global names must not resolve to the query's internal helper names.
class Answer {};
class valid_0 : public Answer {};

namespace example {
class ProtectedCounter : protected Counter {};
using CounterAlias = Counter;
using DerivedAlias = Derived;
using ConstCounter = const Counter;
using CounterPointer = Counter *;
using Scalar = int32_t;
class Incomplete;
template<class T> class TemplateBase {};
using TemplateInt = TemplateBase<int>;
class TemplateDerived : public TemplateInt {};
#if defined(DYLIB_QUERY_PRIVATE)
class ConditionalCounter : private Counter {};
#else
class ConditionalCounter : public Counter {};
#endif
#if __SIZEOF_POINTER__ == 4
class TargetCounter : public Counter {};
#else
class TargetCounter : public Token {};
#endif
}

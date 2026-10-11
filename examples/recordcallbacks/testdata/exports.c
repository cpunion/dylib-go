#include "exports.h"
static int add(int a, int b) { return a + b; }
static int subtract(int a, int b) { return a - b; }
static Callbacks stored;
static Callbacks slot = {0, add};
Callbacks callbacks_factory(int bias) { return (Callbacks){bias, add}; }
Callbacks callbacks_subtract(void) { return (Callbacks){0, subtract}; }
Callbacks callbacks_echo(Callbacks value) { return value; }
int callbacks_apply(Callbacks value, int a, int b) {
    return value.add ? value.bias + value.add(a, b) : -1;
}
int callbacks_pointer(Callbacks *value, int a, int b) {
    return callbacks_apply(*value, a, b);
}
Callbacks *callbacks_slot(void) { return &slot; }
NestedCallbacks callbacks_nested(NestedCallbacks value) { return value; }
int callbacks_nested_apply(NestedCallbacks value, int a, int b) {
    return callbacks_apply(value.items[2], a, b);
}
void callbacks_store(Callbacks value) { stored = value; }
int callbacks_run(int a, int b) { return callbacks_apply(stored, a, b); }
void callbacks_clear(void) { stored = (Callbacks){0, 0}; }

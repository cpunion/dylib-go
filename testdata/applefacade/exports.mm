#import <Foundation/Foundation.h>
#include "exports.h"

static int32_t live_count;

#ifndef DYLIB_FACADE_CLASS
#define DYLIB_FACADE_CLASS DylibFacadeCounter
#endif
@interface DYLIB_FACADE_CLASS : NSObject {
    int32_t _value;
    AppleObserver _observer;
}
- (instancetype)initWithValue:(int32_t)value observer:(AppleObserver)observer;
- (int32_t)add:(int32_t)delta;
@end

@implementation DYLIB_FACADE_CLASS
- (instancetype)initWithValue:(int32_t)value observer:(AppleObserver)observer {
    self = [super init];
    if (self) {
        _value = value;
        _observer = observer;
        ++live_count;
    }
    return self;
}
- (int32_t)add:(int32_t)delta {
    _value = (int32_t)((uint32_t)_value + (uint32_t)delta);
    return _observer ? _observer(_value) : _value;
}
- (void)dealloc {
    if (_observer) _observer(-1);
    --live_count;
}
@end

void *apple_create(int32_t initial, AppleObserver observer) {
    @autoreleasepool {
        return (__bridge_retained void *)[[DYLIB_FACADE_CLASS alloc]
            initWithValue:initial observer:observer];
    }
}
void *apple_retain(void *handle) {
    @autoreleasepool {
        id object = (__bridge id)handle;
        return (__bridge_retained void *)object;
    }
}
void apple_release(void *handle) {
    @autoreleasepool {
        __unused id object = (__bridge_transfer id)handle;
    }
}
int32_t apple_add(void *handle, int32_t delta) {
    @autoreleasepool {
        return handle ? [(__bridge DYLIB_FACADE_CLASS *)handle add:delta] : -10000;
    }
}
int32_t apple_live(void) { return live_count; }

// The C facade owns all Swift representation and reference-count operations.
public typealias AppleObserver = @convention(c) (Int32) -> Int32
private var liveCount: Int32 = 0

private final class Counter {
    var value: Int32
    let observer: AppleObserver?
    init(_ value: Int32, _ observer: AppleObserver?) {
        self.value = value
        self.observer = observer
        liveCount &+= 1
    }
    func add(_ delta: Int32) -> Int32 {
        value &+= delta
        return observer?(value) ?? value
    }
    deinit {
        _ = observer?(-1)
        liveCount &-= 1
    }
}

@_cdecl("apple_create")
public func appleCreate(_ initial: Int32, _ observer: AppleObserver?) -> UnsafeMutableRawPointer {
    Unmanaged.passRetained(Counter(initial, observer)).toOpaque()
}
@_cdecl("apple_retain")
public func appleRetain(_ handle: UnsafeMutableRawPointer?) -> UnsafeMutableRawPointer? {
    guard let handle else { return nil }
    return Unmanaged<Counter>.fromOpaque(handle).retain().toOpaque()
}
@_cdecl("apple_release")
public func appleRelease(_ handle: UnsafeMutableRawPointer?) {
    if let handle { Unmanaged<Counter>.fromOpaque(handle).release() }
}
@_cdecl("apple_add")
public func appleAdd(_ handle: UnsafeMutableRawPointer?, _ delta: Int32) -> Int32 {
    guard let handle else { return -10000 }
    return Unmanaged<Counter>.fromOpaque(handle).takeUnretainedValue().add(delta)
}
@_cdecl("apple_live")
public func appleLive() -> Int32 { liveCount }

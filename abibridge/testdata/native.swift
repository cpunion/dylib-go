import Darwin

@_silgen_name("fixture_i32")
public func add(_ a: Int32, _ b: Int32) -> Int32 { a + b }

@_silgen_name("fixture_i8")
public func narrow(_ a: Int8, _ b: UInt8) -> Int64 { Int64(a) + Int64(b) }

@_silgen_name("fixture_i8_result")
public func narrowResult(_ a: Int32, _ b: Int32) -> Int8 { Int8(truncatingIfNeeded: a + b) }

@_silgen_name("fixture_u8_result")
public func byteResult(_ a: Int32, _ b: Int32) -> UInt8 { UInt8(truncatingIfNeeded: a + b) }

@_silgen_name("fixture_i16")
public func half(_ a: Int16, _ b: UInt16) -> Int16 { Int16(truncatingIfNeeded: Int32(a) + Int32(b)) }

@_silgen_name("fixture_u16")
public func unsignedHalf(_ a: UInt16) -> UInt16 { a &+ 1 }

@_silgen_name("fixture_u32")
public func unsignedWord(_ a: UInt32, _ b: UInt32) -> UInt32 { a &+ b }

@_silgen_name("fixture_u64")
public func wide(_ a: UInt64, _ b: UInt64) -> UInt64 { a ^ b }

@_silgen_name("fixture_f32")
public func single(_ a: Float, _ b: Float) -> Float { a + b }

@_silgen_name("fixture_f64")
public func double(_ a: Double, _ b: Double) -> Double { a + b }

@frozen public struct Mixed {
    public var integer: Int64
    public var floating: Double
}
@_silgen_name("fixture_mixed")
public func mixed(_ value: Mixed, _ integer: Int64, _ floating: Double) -> Mixed {
    Mixed(integer: value.integer + integer, floating: value.floating + floating)
}

@frozen public struct Padded {
    public var byte: UInt8
    public var wide: UInt64
}
@_silgen_name("fixture_padded")
public func padded(_ value: Padded) -> Padded {
    Padded(byte: value.byte &+ 1, wide: value.wide &+ 1)
}

@frozen public struct Tail {
    public var floating: Double
    public var integer: Int32
}
@_silgen_name("fixture_tail")
public func tail(_ value: Tail) -> Tail {
    Tail(floating: value.floating + 1, integer: value.integer + 1)
}

@frozen public struct Large {
    public var a, b, c, d, e: Int64
}
@_silgen_name("fixture_large")
public func large(_ value: Large) -> Large {
    Large(a: value.e + 1, b: value.d + 2, c: value.c + 3, d: value.b + 4, e: value.a + 5)
}

// Library evolution keeps this formally indirect even though storage is small.
public struct Resilient {
    public var a, b: Int32
}
@_silgen_name("fixture_indirect")
public func resilient(_ value: Resilient) -> Resilient {
    Resilient(a: value.a + value.b, b: value.a - value.b)
}

@_silgen_name("fixture_spill")
public func spill(_ a: Int64, _ b: Int64, _ c: Int64, _ d: Int64, _ e: Int64,
                  _ f: Int64, _ g: Int64, _ h: Int64, _ i: Int64, _ j: Int64,
                  _ x: Double, _ y: Double, _ z: Double, _ u: Double, _ v: Double,
                  _ w: Double, _ p: Double, _ q: Double, _ r: Double, _ s: Double) -> Double {
    Double(a+b+c+d+e+f+g+h+i+j) + x+y+z+u+v+w+p+q+r+s
}

@_silgen_name("fixture_allocate")
public func allocate() -> UnsafeMutablePointer<Int32> {
    let pointer = UnsafeMutablePointer<Int32>.allocate(capacity: 1)
    pointer.initialize(to: 20)
    return pointer
}
@_silgen_name("fixture_pointer")
public func pointer(_ value: UnsafeMutablePointer<Int32>, _ amount: Int32) -> Int32 {
    value.pointee += amount
    return value.pointee
}
@_silgen_name("fixture_free")
public func free(_ value: UnsafeMutablePointer<Int32>) {
    value.deinitialize(count: 1)
    value.deallocate()
}

@_silgen_name("fixture_block")
public func block() -> Int32 { fixture_wait() }

@_cdecl("fixture_layout")
public func layout(_ which: Int32, _ index: Int32) -> Int32 {
    switch (which, index) {
    case (0, 0): return Int32(MemoryLayout<Mixed>.size)
    case (0, 1): return Int32(MemoryLayout<Mixed>.alignment)
    case (0, 2): return Int32(MemoryLayout<Mixed>.offset(of: \.floating)!)
    case (1, 0): return Int32(MemoryLayout<Large>.size)
    case (1, 1): return Int32(MemoryLayout<Large>.alignment)
    case (2, 0): return Int32(MemoryLayout<Resilient>.size)
    case (2, 1): return Int32(MemoryLayout<Resilient>.alignment)
    case (3, 0): return Int32(MemoryLayout<Tail>.size)
    case (3, 1): return Int32(MemoryLayout<Tail>.alignment)
    default: return -1
    }
}

private final class ManagedToken { deinit { fixture_value_record() } }
private struct ManagedRecord {
    let token: ManagedToken
    let text: String
    let number: Int64
}
private struct EmptyValue {}
@_cdecl("fixture_managed_metadata")
public func managedMetadata() -> UnsafeRawPointer { unsafeBitCast(ManagedRecord.self, to: UnsafeRawPointer.self) }
@_cdecl("fixture_gated_metadata")
public func gatedMetadata() -> UnsafeRawPointer {
    _ = fixture_wait()
    return managedMetadata()
}
@_cdecl("fixture_empty_metadata")
public func emptyMetadata() -> UnsafeRawPointer { unsafeBitCast(EmptyValue.self, to: UnsafeRawPointer.self) }
@_silgen_name("fixture_managed_initialize")
public func managedInitialize(_ destination: UnsafeMutableRawPointer, _ number: Int64) {
    destination.bindMemory(to: ManagedRecord.self, capacity: 1).initialize(to:
        ManagedRecord(token: ManagedToken(), text: String(repeating: "native", count: 100), number: number))
}
@_silgen_name("fixture_managed_read")
public func managedRead(_ source: UnsafeRawPointer) -> Int64 {
    let value = source.assumingMemoryBound(to: ManagedRecord.self).pointee
    return value.number + Int64(value.text.count)
}
@_silgen_name("fixture_empty_initialize")
public func emptyInitialize(_ destination: UnsafeMutableRawPointer) {
    destination.bindMemory(to: EmptyValue.self, capacity: 1).initialize(to: EmptyValue())
}
@_cdecl("fixture_value_layout")
public func valueLayout(_ which: Int32, _ index: Int32) -> Int32 {
    let layout = which == 0
        ? (MemoryLayout<ManagedRecord>.size, MemoryLayout<ManagedRecord>.stride, MemoryLayout<ManagedRecord>.alignment)
        : (MemoryLayout<EmptyValue>.size, MemoryLayout<EmptyValue>.stride, MemoryLayout<EmptyValue>.alignment)
    switch index { case 0: return Int32(layout.0); case 1: return Int32(layout.1); case 2: return Int32(layout.2); default: return -1 }
}

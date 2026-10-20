private struct Record {
    let text: String
    let number: Int64
}

@_cdecl("fixture_value_example_metadata")
public func metadata() -> UnsafeRawPointer {
    unsafeBitCast(Record.self, to: UnsafeRawPointer.self)
}

@_silgen_name("fixture_value_example_initialize")
public func initialize(_ destination: UnsafeMutableRawPointer, _ number: Int64) {
    destination.bindMemory(to: Record.self, capacity: 1).initialize(
        to: Record(text: String(repeating: "abcdef", count: 100), number: number))
}

@_silgen_name("fixture_value_example_read")
public func read(_ source: UnsafeRawPointer) -> Int64 {
    let value = source.assumingMemoryBound(to: Record.self).pointee
    return Int64(value.text.utf8.count) + value.number
}

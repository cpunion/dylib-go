#![no_std]
#[unsafe(no_mangle)]
pub extern "C" fn rust_add(a: i32, b: i32) -> i32 { a.wrapping_add(b) }

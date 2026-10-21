#![no_std]

#[repr(C)]
#[derive(Clone, Copy)]
pub struct Pair { a: i32, b: f64 }
#[repr(C)]
#[derive(Clone, Copy)]
pub struct Large { a: i64, b: i64, c: i64, d: i64, e: i64 }

macro_rules! entries {
    ($module:ident, $cc:literal, $prefix:literal) => {
        mod $module {
            use super::{Pair, Large};
            type PairEntry = extern $cc fn(Pair, i32) -> Pair;
            type LargeEntry = extern $cc fn(Large, i64) -> Large;
            #[unsafe(export_name = concat!($prefix, "_add"))]
            pub extern $cc fn add(a: i32, b: i32) -> i32 { a.wrapping_add(b) }
            #[unsafe(export_name = concat!($prefix, "_narrow"))]
            pub extern $cc fn narrow(a: i8) -> i8 { a }
            #[unsafe(export_name = concat!($prefix, "_wide"))]
            pub extern $cc fn wide(a: u64) -> u64 { a ^ 0x12345678fedcba98 }
            #[unsafe(export_name = concat!($prefix, "_boolean"))]
            pub extern $cc fn boolean(a: bool) -> bool { !a }
            #[unsafe(export_name = concat!($prefix, "_pointer"))]
            pub extern $cc fn pointer(a: *mut ()) -> *mut () { a }
            #[unsafe(export_name = concat!($prefix, "_mutate"))]
            pub unsafe extern $cc fn mutate(a: *mut i32, n: i32) {
                unsafe { a.write((*a).wrapping_add(n)); }
            }
            #[unsafe(export_name = concat!($prefix, "_mixed"))]
            pub extern $cc fn mixed(a: i8, b: u16, c: f32, d: f64, e: i64) -> f64 {
                a as f64 + b as f64 + c as f64 + d + e as f64
            }
            #[unsafe(export_name = concat!($prefix, "_spill"))]
            pub extern $cc fn spill(a: i32,b: i32,c: i32,d: i32,e: i32,f: i32,g: i32,h: i32,
                i: f64,j: f64,k: f64,l: f64,m: f64,n: f64,o: f64,p: f64,q: f64,r: f64) -> f64 {
                a as f64+b as f64+c as f64+d as f64+e as f64+f as f64+g as f64+h as f64+i+j+k+l+m+n+o+p+q+r
            }
            #[unsafe(export_name = concat!($prefix, "_pair"))]
            pub extern $cc fn pair(mut p: Pair, n: i32) -> Pair {
                p.a = p.a.wrapping_add(n); p.b += n as f64; p
            }
            #[unsafe(export_name = concat!($prefix, "_large"))]
            pub extern $cc fn large(p: Large, n: i64) -> Large {
                Large { a:p.a.wrapping_add(n),b:p.b.wrapping_add(n),c:p.c.wrapping_add(n),d:p.d.wrapping_add(n),e:p.e.wrapping_add(n) }
            }
            #[unsafe(export_name = concat!($prefix, "_factory"))]
            pub extern "C" fn factory() -> PairEntry { pair }
            #[unsafe(export_name = concat!($prefix, "_apply"))]
            pub extern "C" fn apply(entry: PairEntry, p: Pair, n: i32) -> Pair { entry(p,n) }
            #[unsafe(export_name = concat!($prefix, "_forward"))]
            pub extern $cc fn forward(entry: PairEntry, p: Pair, n: i32) -> Pair { entry(p,n) }
            #[unsafe(export_name = concat!($prefix, "_apply_large"))]
            pub extern "C" fn apply_large(entry: LargeEntry, p: Large, n: i64) -> Large { entry(p,n) }
        }
    }
}
entries!(microsoft, "win64", "ms");
entries!(system_v, "sysv64", "sysv");

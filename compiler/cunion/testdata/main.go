package main

import (
	"fmt"
	dylib "github.com/cpunion/dylib-go"
	"math"
	"os"
	"strings"
	"unsafe"
)

func check[T comparable](value T, err error, want T) {
	if err != nil || value != want {
		panic(fmt.Sprintf("union call: %v, %v; want %v", value, err, want))
	}
}
func main() {
	s := dylib.New(dylib.Options{})
	defer s.Close()
	for _, path := range strings.Split(os.Args[1], "\n") {
		if err := s.Load(path); err != nil {
			panic(err)
		}
	}
	b, err := NewBindings(s)
	if err != nil {
		panic(err)
	}
	v, e := b.Union_add(20, 22)
	check(v, e, int32(42))
	d, e := b.Union_double(42.5)
	check(d, e, 42.5)
	i8, e := b.Union_i8(-128)
	check(i8, e, int8(-128))
	u8, e := b.Union_u8(255)
	check(u8, e, uint8(255))
	i16, e := b.Union_i16(-32768)
	check(i16, e, int16(-32768))
	u16, e := b.Union_u16(65535)
	check(u16, e, uint16(65535))
	i32, e := b.Union_i32(-2147483648)
	check(i32, e, int32(-2147483648))
	u32, e := b.Union_u32(4294967295)
	check(u32, e, uint32(4294967295))
	i64, e := b.Union_i64(-9223372036854775808)
	check(i64, e, int64(-9223372036854775808))
	u64, e := b.Union_u64(18446744073709551615)
	check(u64, e, uint64(18446744073709551615))
	f, e := b.Union_f32(-42.5)
	check(f, e, float32(-42.5))
	boolean, e := b.Union_bool(true)
	check(boolean, e, true)
	boolean, e = b.Union_bool(false)
	check(boolean, e, false)
	d, e = b.Union_double(math.Float64frombits(0x7ff8000000000042))
	if e != nil || math.Float64bits(d) != 0x7ff8000000000042 {
		panic("NaN payload")
	}
	d, e = b.Union_double(math.Copysign(0, -1))
	if e != nil || math.Float64bits(d) != 1<<63 {
		panic("negative zero")
	}
	f, e = b.Union_small(20, 22)
	check(f, e, float32(42))
	v, e = b.Union_large(20, 22)
	check(v, e, int32(42))
	d, e = b.Union_packed(20.5, 21.5)
	check(d, e, 42.0)
	v, e = b.Union_aligned(20, 22)
	check(v, e, int32(42))
	if err := b.Union_store(42); err != nil {
		panic(err)
	}
	v, e = b.Union_stored()
	check(v, e, int32(42))
	v, e = b.Union_read(42)
	check(v, e, int32(42))
	address, e := b.Union_address()
	if e != nil || address == nil {
		panic("native address")
	}
	p, e := b.Union_pointer(address)
	check(p, e, address)
	p, e = b.Union_pointer(nil)
	check(p, e, unsafe.Pointer(nil))
	v, e = b.Union_pointer_read(address)
	check(v, e, int32(42))
	if err := s.Close(); err != nil {
		panic(err)
	}
	if _, err := b.Union_add(20, 22); err != dylib.ErrClosed {
		panic("closed union provider")
	}
	fmt.Println(42)
}

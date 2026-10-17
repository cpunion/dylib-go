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
	pair, e := b.Union_pair_make(20, 22)
	if e != nil {
		panic(e)
	}
	zero, e := b.Union_pair_make(0, 0)
	if e != nil {
		panic(e)
	}
	pairResult, e := b.Union_pair(pair, zero)
	check(pairResult, e, pair)
	pairResult, e = b.Union_pair(pair, pair)
	if e != nil || pairResult.A != 40 || pairResult.B != 44 {
		panic("union struct member")
	}
	bundle, e := b.Union_bundle_make(20, 22)
	if e != nil {
		panic(e)
	}
	bundleResult, e := b.Union_bundle(bundle)
	check(bundleResult, e, bundle)
	if bundleResult.Pair.A != 20 || bundleResult.Rows[1][0] != 22 || bundleResult.Values[1] != 21.5 || bundleResult.Pointer != address {
		panic("nested union struct member")
	}
	hfa, e := b.Union_hfa_make(20.5, 21.5)
	if e != nil {
		panic(e)
	}
	hfaResult, e := b.Union_hfa(hfa)
	check(hfaResult, e, hfa)
	if hfaResult.Values[0]+hfaResult.Values[1] != 42 {
		panic("union HFA member")
	}
	v, e = b.Union_nested_add(20, 22)
	check(v, e, int32(42))
	v, e = b.Union_nested_small_add(20, 22)
	check(v, e, int32(42))
	v, e = b.Union_nested_add(-64, 22)
	check(v, e, int32(-42))
	pairResult, e = b.Union_nested_pair(pair)
	check(pairResult, e, pair)
	bundleResult, e = b.Union_nested_bundle(bundle)
	check(bundleResult, e, bundle)
	hfaResult, e = b.Union_nested_hfa(hfa)
	check(hfaResult, e, hfa)
	d, e = b.Union_nested_double(42.5)
	check(d, e, 42.5)
	p, e = b.Union_nested_pointer(address)
	check(p, e, address)
	p, e = b.Union_nested_pointer(nil)
	check(p, e, unsafe.Pointer(nil))
	ints, e := b.Union_array_ints_make(20, 22)
	if e != nil || ints.Values != [2]int32{20, 22} {
		panic("array factory")
	}
	intsResult, e := b.Union_array_ints(ints)
	check(intsResult, e, ints)
	intsResult, e = b.Union_arrays_add(ints, 22)
	if e != nil || intsResult.Values != [2]int32{42, 44} {
		panic("array union argument/result")
	}
	intsResult, e = b.Union_array_small(ints, 22)
	if e != nil || intsResult.Values != [2]int32{42, 44} {
		panic("small array union")
	}
	intsResult, e = b.Union_array_aligned(ints)
	check(intsResult, e, ints)
	intsResult, e = b.Union_array_nested(ints)
	check(intsResult, e, ints)
	matrix, e := b.Union_array_matrix_make(20, 22)
	if e != nil || matrix.Values != [2][2]int32{{20, 22}, {22, 20}} {
		panic("multidimensional array")
	}
	matrixResult, e := b.Union_array_matrix(matrix)
	check(matrixResult, e, matrix)
	pairs, e := b.Union_array_pairs_make(20, 22)
	if e != nil || pairs.Values[0].A != 20 || pairs.Values[1].A != 22 {
		panic("record array")
	}
	pairsResult, e := b.Union_array_pairs(pairs)
	check(pairsResult, e, pairs)
	doubles, e := b.Union_array_hfa_make(20.5, 21.5)
	if e != nil || doubles.Values != [2]float64{20.5, 21.5} {
		panic("floating array")
	}
	doublesResult, e := b.Union_array_hfa(doubles)
	check(doublesResult, e, doubles)
	doublesResult, e = b.Union_array_packed(doubles)
	check(doublesResult, e, doubles)
	pointers, e := b.Union_array_pointers_make()
	if e != nil || pointers.Values != [2]unsafe.Pointer{nil, address} {
		panic("pointer array")
	}
	pointersResult, e := b.Union_array_pointers(pointers)
	check(pointersResult, e, pointers)
	smallBytes, e := b.Union_bytes_small_make(17)
	if e != nil {
		panic(e)
	}
	for i, v := range smallBytes.Values {
		if v != byte((17+17*i)%256) {
			panic("union byte factory")
		}
	}
	smallWant := smallBytes
	for i := range smallWant.Values {
		smallWant.Values[i] ^= 0x5a
	}
	smallResult, e := b.Union_bytes_small(smallBytes)
	check(smallResult, e, smallWant)

	largeBytes, e := b.Union_bytes_large_make(17)
	if e != nil {
		panic(e)
	}
	for i, v := range largeBytes.Values {
		if v != byte((17+17*i)%256) {
			panic("union byte factory")
		}
	}
	largeWant := largeBytes
	for i := range largeWant.Values {
		largeWant.Values[i] ^= 0x5a
	}
	largeResult, e := b.Union_bytes_large(largeBytes)
	check(largeResult, e, largeWant)

	packedBytes, e := b.Union_bytes_packed_make(17)
	if e != nil {
		panic(e)
	}
	for i, v := range packedBytes.Values {
		if v != byte((17+17*i)%256) {
			panic("union byte factory")
		}
	}
	packedWant := packedBytes
	for i := range packedWant.Values {
		packedWant.Values[i] ^= 0x5a
	}
	packedResult, e := b.Union_bytes_packed(packedBytes)
	check(packedResult, e, packedWant)

	alignedBytes, e := b.Union_bytes_aligned_make(17)
	if e != nil {
		panic(e)
	}
	for i, v := range alignedBytes.Values {
		if v != byte((17+17*i)%256) {
			panic("union byte factory")
		}
	}
	alignedWant := alignedBytes
	for i := range alignedWant.Values {
		alignedWant.Values[i] ^= 0x5a
	}
	alignedResult, e := b.Union_bytes_aligned(alignedBytes)
	check(alignedResult, e, alignedWant)

	nestedBytes, e := b.Union_bytes_nested_make(17)
	if e != nil {
		panic(e)
	}
	for i, v := range nestedBytes.Values {
		if v != byte((17+17*i)%256) {
			panic("union byte factory")
		}
	}
	nestedWant := nestedBytes
	for i := range nestedWant.Values {
		nestedWant.Values[i] ^= 0x5a
	}
	nestedResult, e := b.Union_bytes_nested(nestedBytes)
	check(nestedResult, e, nestedWant)

	if err := s.Close(); err != nil {
		panic(err)
	}
	if _, err := b.Union_add(20, 22); err != dylib.ErrClosed {
		panic("closed union provider")
	}
	if _, err := b.Union_bytes_small(smallBytes); err != dylib.ErrClosed {
		panic("closed union byte view")
	}
	fmt.Println(42)
}

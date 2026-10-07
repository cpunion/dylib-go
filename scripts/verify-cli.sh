#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
compiler=${1:?usage: verify-cli.sh go|llgo}
case "$compiler" in
  go|llgo) ;;
  *) echo "unsupported compiler: $compiler" >&2; exit 1 ;;
esac
cli_dir=$(mktemp -d)
trap 'rm -rf "$cli_dir"' EXIT
binary="$cli_dir/ddlgo"
case "$(uname -s)" in
  Darwin)
    library="$cli_dir/calls.dylib"
    "${CLANG:-clang}" -dynamiclib testdata/add.c testdata/scalars.c testdata/cli.c -o "$library"
    ;;
  Linux)
    library="$cli_dir/calls.so"
    "${CLANG:-clang}" -shared -fPIC testdata/add.c testdata/scalars.c testdata/cli.c -o "$library"
    ;;
  MINGW*|MSYS*)
    binary="$cli_dir/ddlgo.exe"
    library="$cli_dir/calls.dll"
    "${CLANG:-clang}" -shared testdata/add.c testdata/scalars.c testdata/cli.c -Wl,--export-all-symbols -o "$library"
    ;;
  *) echo 'unsupported CLI test host' >&2; exit 1 ;;
esac
"$compiler" build -tags libffi -o "$binary" ./cmd/ddlgo
expect() {
  local expected=$1
  shift
  local result
  result=$("$binary" call "$@")
  test "$result" = "$expected"
  echo "$compiler CLI $1: $result"
}
expect 42 'func add(int32,int32)int32' 20 22 "$library"
expect 42 'add(20:int32 22:int32)int32' "$library"
expect 42 'add(20:int32, 22:int32)int32' "$library"
expect 42 add 20 22 "$library"
expect 42 'func mixed(int32,float64,float32,uint64)float64' 10 20.5 1.5 10 "$library"
expect 42 'mixed(10:int32,20.5:float64,1.5:float32,10:uint64)float64' "$library"
expect -42 'func negate(int32)int32' 42 "$library"
expect 4294967295 'echo_u32(0xffffffff:uint32)uint32' "$library"
expect -9223372036854775808 'echo_i64(-9223372036854775808:int64)int64' "$library"
expect 18446744073709551615 'echo_u64(18446744073709551615:uint64)uint64' "$library"
expect 20.5 'echo_f32(20.5:float32)float32' "$library"
expect 0x0 'echo_ptr(nil:unsafe.Pointer)unsafe.Pointer' "$library"
expect 42 'func answer()int32' "$library"
expect void 'no_result()' "$library"
expect -128 'echo_i8(-128:int8)int8' "$library"
expect 255 'echo_u8(255:uint8)uint8' "$library"
expect -32768 'echo_i16(-32768:int16)int16' "$library"
expect 65535 'echo_u16(65535:uint16)uint16' "$library"
expect true 'echo_bool(true:bool)bool' "$library"
expect 42 'func sum_pair(struct{a,b int32})int32' '{a:20,b:22}' "$library"
expect 42 'sum_pair({a:20,b:22}:struct{a,b int32})int32' "$library"
expect 42 'sum_pair_ptr(&{a:20,b:22}:*struct{a,b int32})int32' "$library"
expect '{a:20,b:22}' 'echo_pair({20,22}:struct{a,b int32})struct{a,b int32}' "$library"
expect '&{a:22,b:20}' 'mutate_pair({20,22}:*struct{a,b int32})*struct{a,b int32}' "$library"
expect 42 'scalar_ptr(&40:*int32)int32' "$library"
expect 42 'sum_nested({tag:1,p:{a:20,b:20},extra:1}:struct{tag int8;p struct{a,b int32};extra float64})float64' "$library"
expect '{tag:1,p:{a:20,b:20},extra:1}' 'echo_nested({tag:1,p:{a:20,b:20},extra:1}:struct{tag int8;p struct{a,b int32};extra float64})struct{tag int8;p struct{a,b int32};extra float64}' "$library"
expect '{x:20.5,y:21.5}' 'echo_float_pair({20.5,21.5}:struct{x,y float32})struct{x,y float32}' "$library"
expect '{p:&{a:22,b:20},bonus:0}' 'echo_pair_ref({p:&{a:20,b:20}}:struct{p *struct{a,b int32};bonus int32})struct{p *struct{a,b int32};bonus int32}' "$library"

# Dynamic declarations also execute raw objects/archives on the five supported
# targets. Windows ARM64 deliberately uses the DLL path only.
if [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]]; then
  if [[ "$(go env GOARCH)" == arm64 ]]; then
    echo 'Windows ARM64: DLL calls verified; raw COFF remains unsupported'
    exit 0
  fi
  "${CLANG:-clang}" --target=x86_64-pc-windows-msvc -ffreestanding -c testdata/add.c -o "$cli_dir/add.o"
  "${CLANG:-clang}" --target=x86_64-pc-windows-msvc -ffreestanding -c testdata/pair.c -o "$cli_dir/pair.o"
else
  "${CLANG:-clang}" -fPIC -c testdata/add.c -o "$cli_dir/add.o"
  "${CLANG:-clang}" -fPIC -c testdata/pair.c -o "$cli_dir/pair.o"
fi
"${AR:-ar}" rcs "$cli_dir/add.a" "$cli_dir/add.o"
"${AR:-ar}" rcs "$cli_dir/pair.a" "$cli_dir/pair.o"
expect 42 'func add(int32,int32)int32' 20 22 "$cli_dir/add.o"
expect 42 'add(20:int32 22:int32)int32' "$cli_dir/add.a"
expect 42 'sum_pair({20,22}:struct{a,b int32})int32' "$cli_dir/pair.o"
expect '{a:20,b:22}' 'echo_pair({20,22}:struct{a,b int32})struct{a,b int32}' "$cli_dir/pair.a"
expect '&{a:22,b:20}' 'mutate_pair(&{20,22}:*struct{a,b int32})*struct{a,b int32}' "$cli_dir/pair.o"

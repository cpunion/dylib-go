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

# Dynamic declarations also execute raw objects/archives on the five supported
# targets. Windows ARM64 deliberately uses the DLL path only.
if [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]]; then
  if [[ "$(go env GOARCH)" == arm64 ]]; then
    echo 'Windows ARM64: DLL calls verified; raw COFF remains unsupported'
    exit 0
  fi
  "${CLANG:-clang}" --target=x86_64-pc-windows-msvc -ffreestanding -c testdata/add.c -o "$cli_dir/add.o"
else
  "${CLANG:-clang}" -fPIC -c testdata/add.c -o "$cli_dir/add.o"
fi
"${AR:-ar}" rcs "$cli_dir/add.a" "$cli_dir/add.o"
expect 42 'func add(int32,int32)int32' 20 22 "$cli_dir/add.o"
expect 42 'add(20:int32 22:int32)int32' "$cli_dir/add.a"

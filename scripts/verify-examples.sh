#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
compiler=${1:?usage: verify-examples.sh go|llgo}
case "$compiler" in
  go) example=cgo ;;
  llgo) example=llgo ;;
  *) echo "unsupported compiler: $compiler" >&2; exit 1 ;;
esac
example_dir=$(mktemp -d)
trap 'rm -rf "$example_dir"' EXIT
source scripts/native-cflags.sh
binary="$example_dir/mixed"
case "$(uname -s)" in
  Darwin)
    library="$example_dir/scalars.dylib"
    "${CLANG:-clang}" -dynamiclib testdata/scalars.c -o "$library"
    ;;
  Linux)
    library="$example_dir/scalars.so"
    "${CLANG:-clang}" "${native_cflags[@]}" -shared -fPIC testdata/scalars.c -o "$library"
    ;;
  MINGW*|MSYS*)
    binary="$example_dir/mixed.exe"
    library="$example_dir/scalars.dll"
    "${CLANG:-clang}" "${native_cflags[@]}" -shared testdata/scalars.c -Wl,--export-all-symbols -o "$library"
    ;;
  *) echo 'unsupported example host' >&2; exit 1 ;;
esac
"$compiler" build -o "$binary" "./examples/$example"
result=$("$binary" "$library")
test "$result" = 42
echo "$compiler caller-defined mixed C signature: $result"

"$compiler" build -tags libffi -o "$binary" ./examples/bind
result=$("$binary" "$library")
test "$result" = 42
echo "$compiler dynamic libffi signature: $result"

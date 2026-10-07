#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
compiler=${1:-go} # Pass llgo to build with llgo instead.
mkdir -p build
binary=build/ddlgo
case "$(uname -s)" in
  Darwin)
    library=build/structs.dylib
    "${CLANG:-clang}" -dynamiclib testdata/cli.c -o "$library"
    ;;
  Linux)
    library=build/structs.so
    "${CLANG:-clang}" -shared -fPIC testdata/cli.c -o "$library"
    ;;
  MINGW*|MSYS*)
    binary=build/ddlgo.exe
    library=build/structs.dll
    "${CLANG:-clang}" -shared testdata/cli.c -Wl,--export-all-symbols -o "$library"
    ;;
  *) echo 'unsupported example host' >&2; exit 1 ;;
esac
"$compiler" build -tags libffi -o "$binary" ./cmd/ddlgo

# Pass an ordinary C struct by value using a Go-style declaration.
result=$("$binary" call "func sum_pair(struct{a,b int32})int32" "{a:20,b:22}" "$library")
test "$result" = 42
echo "struct value result: $result"

# The known pointee type allows a temporary native copy of this literal.
result=$("$binary" call "sum_pair_ptr(&{a:20,b:22}:*struct{a,b int32})int32" "$library")
test "$result" = 42
echo "struct pointer result: $result"

# Mutations are copied back; a returned temporary pointer becomes a snapshot.
result=$("$binary" call "mutate_pair(&{20,22}:*struct{a,b int32})*struct{a,b int32}" "$library")
test "$result" = '&{a:22,b:20}'
echo "modified struct result: $result"

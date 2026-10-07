#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
compiler=${1:-go} # Pass llgo to build the CLI with its direct C-call adapter.
case "$compiler" in
  go|llgo) ;;
  *) echo 'usage: quickstart.sh [go|llgo]' >&2; exit 1 ;;
esac

mkdir -p build
source scripts/native-cflags.sh
"${CLANG:-clang}" ${native_cflags[@]+"${native_cflags[@]}"} -fPIC -c testdata/add.c -o build/add.o
"$compiler" build -o build/ddlgo ./cmd/ddlgo
build/ddlgo inspect build/add.o
result=$(build/ddlgo call add 20 22 build/add.o)
test "$result" = 42
echo "object result: $result"

"${AR:-ar}" rcs build/add.a build/add.o
result=$(build/ddlgo call add 20 22 build/add.a)
test "$result" = 42
echo "archive result: $result"

# Metadata inspection also works without cgo.
CGO_ENABLED=0 go run ./cmd/ddlgo inspect build/add.o

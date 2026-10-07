#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
compiler=${1:-go} # Pass llgo to build the same dynamic CLI with llgo.

mkdir -p build
"${CLANG:-clang}" -fPIC -c testdata/add.c -o build/add.o
"$compiler" build -tags libffi -o build/ddlgo ./cmd/ddlgo

result=$(build/ddlgo call "func add(int32,int32)int32" 20 22 build/add.o)
test "$result" = 42
echo "declaration result: $result"

# Quote the whole invocation so parentheses and spaces reach the CLI.
result=$(build/ddlgo call "add(20:int32 22:int32)int32" build/add.o)
test "$result" = 42
echo "typed invocation result: $result"

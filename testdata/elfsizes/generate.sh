#!/usr/bin/env bash
set -euo pipefail
fixture_dir=$(cd "$(dirname "$0")" && pwd)
temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT
for arch in amd64 386; do
  triple=x86_64-linux-gnu
  mode=--64
  if [[ "$arch" == 386 ]]; then
    triple=i686-linux-gnu
    mode=--32
  fi
  "${CLANG:-clang}" --target="$triple" -E -P -x assembler-with-cpp \
    "$fixture_dir/../elf_sizes.S" -o "$temporary_dir/sizes.s"
  "${GNU_AS:-as}" "$mode" "$temporary_dir/sizes.s" -o "$fixture_dir/linux_$arch.o"
done

#!/usr/bin/env bash
set -euo pipefail
# Fetch immutable optional native dependencies; Go packages build independently.
directory=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/dylib-abibridge-sources.XXXXXX")
checkout() {
  local name=$1 remote=$2 revision=$3
  git init -q "$directory/$name"
  git -C "$directory/$name" remote add origin "$remote"
  git -C "$directory/$name" fetch -q --depth=1 origin "$revision"
  git -C "$directory/$name" checkout -q --detach FETCH_HEAD
  test "$(git -C "$directory/$name" rev-parse HEAD)" = "$revision"
}
checkout core https://github.com/lynnswap/ABIBridge.git 4dfbda22afb9a1492e1c5c99933a9defbc5437a4
checkout ffi https://github.com/lynnswap/ZDLibffi.git e9737c755088a4a3d2ba03f2ca5bfeadff9c09b2
export DYLIB_ABIBRIDGE_SOURCE="$directory/core"
export DYLIB_ABIBRIDGE_LIBFFI_SOURCE="$directory/ffi"
{
  echo "DYLIB_ABIBRIDGE_SOURCE=$directory/core"
  echo "DYLIB_ABIBRIDGE_LIBFFI_SOURCE=$directory/ffi"
  echo 'DYLIB_TEST_REQUIRE_ABIBRIDGE=1'
} >> "$GITHUB_ENV"
xcrun swift --version
go run ./internal/abibridgefixture >> "$GITHUB_ENV"

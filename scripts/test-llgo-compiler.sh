#!/usr/bin/env bash
set -euo pipefail

# Keep the expanded compiler matrix in bounded processes on Intel macOS.
# Compile once and enumerate the binary's real tests so new tests are included.
directory=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/dylib-compiler.XXXXXX")
trap 'rm -rf "$directory"' EXIT
binary="$directory/clang.test"
llgo test -c -tags libffi -o "$binary" ./compiler/clang
cd compiler/clang
"$binary" -test.list '^Test' > "$directory/tests"
tests=()
while IFS= read -r name; do
  case "$name" in
    Test*) tests+=("$name") ;;
  esac
done < "$directory/tests"
if (( ${#tests[@]} == 0 )); then
  echo 'Compiler test binary listed no tests' >&2
  exit 1
fi
for ((batch = 0; batch < 4; batch++)); do
  pattern=
  count=0
  for ((index = batch; index < ${#tests[@]}; index += 4)); do
    pattern+="${pattern:+|}${tests[index]}"
    count=$((count + 1))
  done
  if (( count == 0 )); then continue; fi
  printf 'Compiler qualification batch %d/4 (%d tests)\n' "$((batch + 1))" "$count"
  "$binary" -test.count=1 -test.timeout="${LLGO_TEST_TIMEOUT:-35m}" \
    -test.run="^($pattern)$" -test.v
done

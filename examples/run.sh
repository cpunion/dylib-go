#!/usr/bin/env bash
set -euo pipefail
repo_root=$(cd "$(dirname "$0")/.." && pwd)

usage() {
  echo 'usage: examples/run.sh <go|llgo> {call ARGS...|inspect FILE|llvm MODULE|library|quickstart|dynamic|structs}' >&2
  exit 1
}

[[ $# -ge 2 ]] || usage
compiler=$1
action=$2
shift 2
case "$compiler" in
  go|llgo) ;;
  *) usage ;;
esac

case "$action" in
  call|inspect|llvm)
    [[ $# -ge 1 ]] || usage
    cli_dir=$(mktemp -d)
    trap 'rm -rf "$cli_dir"' EXIT
    binary="$cli_dir/ddlgo"
    case "$(uname -s)" in
      MINGW*|MSYS*) binary="$binary.exe" ;;
    esac
    build_args=(-o "$binary" ./cmd/ddlgo)
    if [[ "$action" == call || "$action" == llvm ]]; then
      build_args=(-tags libffi "${build_args[@]}")
    fi
    if [[ "$action" == llvm ]]; then
      [[ $# -eq 1 ]] || usage
      build_args=(-tags libffi -o "$binary" ./examples/llvm)
    fi
    (cd "$repo_root" && "$compiler" build "${build_args[@]}" >&2)
    if [[ "$action" == llvm ]]; then
      "$binary" "$@"
      exit $?
    fi
    "$binary" "$action" "$@"
    exit $?
    ;;
  library) script=scripts/verify-examples.sh ;;
  quickstart) script=examples/readme/quickstart.sh ;;
  dynamic) script=examples/readme/dynamic.sh ;;
  structs) script=examples/readme/structs.sh ;;
  *) usage ;;
esac

[[ $# -eq 0 ]] || usage
exec bash "$repo_root/$script" "$compiler"

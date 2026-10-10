#!/usr/bin/env bash
set -euo pipefail
repo_root=$(cd "$(dirname "$0")/.." && pwd)

usage() {
  echo 'usage: examples/run.sh <go|llgo> {call ARGS...|inspect FILE|llvm MODULE|llvmarchive ARCHIVE|llvmmodules MODULE...|dependencies PROVIDER CONSUMER|declarations LIBRARY|cgodeclarations LIBRARY|cgorecords LIBRARY|directdeclarations LIBRARY|library|quickstart|dynamic|structs}' >&2
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
  call|inspect|llvm|llvmarchive|llvmmodules|dependencies|declarations|cgodeclarations|cgorecords|directdeclarations)
    [[ $# -ge 1 ]] || usage
    cli_dir=$(mktemp -d)
    trap 'rm -rf "$cli_dir"' EXIT
    binary="$cli_dir/ddlgo"
    case "$(uname -s)" in
      MINGW*|MSYS*) binary="$binary.exe" ;;
    esac
    example=""
    case "$action" in
      llvm|llvmarchive|llvmmodules|dependencies|declarations|cgodeclarations|cgorecords|directdeclarations) example="./examples/$action" ;;
    esac
    if [[ -n "$example" ]]; then
      if [[ "$action" == dependencies ]]; then
        [[ $# -eq 2 ]] || usage
      else
        [[ "$action" == llvmmodules || $# -eq 1 ]] || usage
      fi
      [[ "$action" != directdeclarations || "$compiler" == llgo ]] || usage
      build_args=(-o "$binary" "$example")
    else
      build_args=(-o "$binary" ./cmd/ddlgo)
    fi
    if [[ "$action" != inspect && "$action" != directdeclarations && "$action" != cgodeclarations && "$action" != cgorecords ]]; then
      build_args=(-tags libffi "${build_args[@]}")
    fi
    (cd "$repo_root" && "$compiler" build "${build_args[@]}" >&2)
    if [[ -n "$example" ]]; then
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

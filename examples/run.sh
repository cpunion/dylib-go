#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

usage() {
  echo 'usage: bash examples/run.sh {library|quickstart|dynamic|structs} [go|llgo]' >&2
  exit 1
}

[[ $# -ge 1 && $# -le 2 ]] || usage
example=$1
compiler=${2:-go}
case "$compiler" in
  go|llgo) ;;
  *) usage ;;
esac

case "$example" in
  library) script=scripts/verify-examples.sh ;;
  quickstart) script=examples/readme/quickstart.sh ;;
  dynamic) script=examples/readme/dynamic.sh ;;
  structs) script=examples/readme/structs.sh ;;
  *) usage ;;
esac

exec bash "$script" "$compiler"

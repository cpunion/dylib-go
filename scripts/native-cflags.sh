# Source this file from the repository root before compiling native fixtures.
# Bash 3.2 treats an empty array as unset with nounset. Callers use
# ${native_cflags[@]+"${native_cflags[@]}"} to preserve zero or more flags.
native_cflags=()
case "$(go env GOOS)/$(go env GOARCH)" in
  linux/386) native_cflags=(-m32) ;;
  windows/386)
    native_cflags=(--target=i686-w64-windows-gnu)
    if [[ -n "${DYLIB_NATIVE_SYSROOT:-}" ]]; then
      native_cflags+=(--sysroot="$DYLIB_NATIVE_SYSROOT" --rtlib=libgcc)
    fi
    ;;
esac

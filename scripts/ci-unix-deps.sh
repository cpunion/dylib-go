#!/usr/bin/env bash
set -euo pipefail
# Native test producers and the optional scalar ABI backend.
case "$(uname -s)" in
  Linux)
    sudo apt-get update
    sudo apt-get install -y clang binutils libffi-dev pkg-config gfortran
    ;;
  Darwin)
    for formula in libffi pkgconf; do
      brew list --versions "$formula" >/dev/null || brew install "$formula"
    done
    echo "PKG_CONFIG_PATH=$(brew --prefix libffi)/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}" >> "$GITHUB_ENV"
    # Runner images may provide a versioned gfortran executable only.
    fc=$(find "$(brew --prefix)/bin" -maxdepth 1 -name 'gfortran*' | sort | head -n 1)
    if [[ -z "$fc" ]]; then
      brew install gcc
      fc=$(find "$(brew --prefix)/bin" -maxdepth 1 -name 'gfortran*' | sort | head -n 1)
    fi
    test -n "$fc"
    echo "DYLIB_FC=$fc" >> "$GITHUB_ENV"
    ;;
  *) exit 1 ;;
esac

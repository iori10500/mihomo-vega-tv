#!/bin/sh
set -eu
cd "$(dirname "$0")"
if [ ! -x build/private/toolchain/armv7/mac/bin/armv7a-linux-gnueabihf-clang ] || [ ! -d build/private/sysroot-armv7-release ]; then
    # Let the Vega CLI provision its pinned cross-compiler and sysroot first.
    MIHOMO_BOOTSTRAP=1 npm run package:vega
fi
./build-core.sh
npm run package:vega

#!/bin/sh
set -eu
project=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
revision=0455d0d28c4526937380505153e65a843d8612e8
version=v1.19.34
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
if [ -z "${MIHOMO_SOURCE:-}" ]; then
    git clone --depth=1 --branch "$version" https://github.com/MetaCubeX/mihomo.git "$work/source"
    MIHOMO_SOURCE="$work/source"
fi
test "$(git -C "$MIHOMO_SOURCE" rev-parse HEAD)" = "$revision"
cd "$work"
cp "$project"/core/*.go .
cp "$MIHOMO_SOURCE/go.mod" go.mod
cp "$MIHOMO_SOURCE/go.sum" go.sum
ln -s "$MIHOMO_SOURCE" upstream
go mod edit -module=firetv.local/mihomo-bridge -require=github.com/metacubex/mihomo@v0.0.0 -replace=github.com/metacubex/mihomo=./upstream
if [ "${1:-}" = "--test" ]; then
    ln -s "$project/assets" assets
    go test -mod=mod -tags 'netgo osusergo' .
    exit
fi
compiler="$project/build/private/toolchain/armv7/mac/bin/armv7a-linux-gnueabihf-clang"
sysroot="$project/build/private/sysroot-armv7-release"
export GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1
resource=$(find "$project/build/private/toolchain/armv7/mac/armv7/lib/clang" -mindepth 1 -maxdepth 1 -type d | head -1)
crt=$(dirname "$(find -L "$sysroot/usr/lib/arm-oe-linux-gnueabi" -name crtbeginS.o | head -1)")
export CGO_CFLAGS="-O2 -mstackrealign"
export CC="$compiler -Qunused-arguments --sysroot=$sysroot -resource-dir=$resource -B$crt -L$resource/lib/linux/arm"
export AR="$project/build/private/toolchain/armv7/mac/bin/llvm-ar"
mkdir -p "$project/native" "$project/assets/raw"
# ARM's external C calling convention requires an eight-byte-aligned stack.
# The Go 1.26 ARM shared-library constructor reserves a four-byte link register
# in addition to its local frame. Align that frame before calling pthreads.
python3 - "$(go env GOROOT)/src/runtime/asm_arm.s" "$work" <<'PYTHON'
import json, pathlib, sys
source, work = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
original = source.read_text()
old = 'TEXT _rt0_arm_lib(SB),NOSPLIT,$104'
if original.count(old) != 1:
    raise SystemExit('Review ARM constructor alignment for this Go version')
patched = work / 'runtime' / 'asm_arm.s'
patched.parent.mkdir()
patched.write_text(original.replace(old, 'TEXT _rt0_arm_lib(SB),NOSPLIT,$108'))
(work / 'overlay.json').write_text(json.dumps({'Replace': {str(source): str(patched)}}))
PYTHON
go build -overlay="$work/overlay.json" -mod=mod -tags 'netgo osusergo' -buildmode=c-shared -trimpath -ldflags "-buildid= -B none -extar=$AR -s -w -X github.com/metacubex/mihomo/constant.Version=$version" -o "$work/libtvcore.so" .
test -s "$work/libtvcore.so"
install -m 0644 "$work/libtvcore.so" "$project/native/libtvcore.so"
install -m 0644 "$MIHOMO_SOURCE/LICENSE" "$project/assets/raw/Mihomo-LICENSE"
python3 "$project/tools/package_notices.py" "$project"

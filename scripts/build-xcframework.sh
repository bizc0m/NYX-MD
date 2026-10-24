#!/bin/zsh
# Compile le moteur Go en bibliothèque statique universelle (arm64 + x86_64, macOS 13+)
# et l'emballe dans artifacts/CNyxMD.xcframework, utilisé par le paquet Swift.
# À relancer après toute modification de nyxmd/ ou cabi/, puis commiter l'artefact.
set -euo pipefail
cd "${0:A:h}/.."

OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT
export CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS="-mmacosx-version-min=13.0" CGO_LDFLAGS="-mmacosx-version-min=13.0"
for arch in arm64 amd64; do
  clang_arch=$arch; [[ $arch == amd64 ]] && clang_arch=x86_64
  CC="clang -arch $clang_arch" GOOS=darwin GOARCH=$arch \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -buildmode=c-archive -o "$OUT/$arch/libnyxmd.a" ./cabi
done
lipo -create "$OUT/arm64/libnyxmd.a" "$OUT/amd64/libnyxmd.a" -output "$OUT/libnyxmd.a"
mkdir -p "$OUT/headers"
cp include/nyxmd.h include/module.modulemap "$OUT/headers/"
rm -rf artifacts/CNyxMD.xcframework
xcodebuild -create-xcframework -library "$OUT/libnyxmd.a" -headers "$OUT/headers" \
  -output artifacts/CNyxMD.xcframework >/dev/null
./scripts/source-hash.sh > artifacts/SOURCE_SHA256
echo "OK: artifacts/CNyxMD.xcframework ($(du -sh artifacts/CNyxMD.xcframework | cut -f1))"
lipo -info artifacts/CNyxMD.xcframework/*/libnyxmd.a

#!/bin/sh
# Empreinte des sources compilées dans CNyxMD.xcframework (Go + en-têtes C).
# La CI la compare à artifacts/SOURCE_SHA256 pour détecter un artefact non reconstruit.
cd "$(dirname "$0")/.."
find go.mod nyxmd cabi include -type f \( -name '*.go' -o -name '*.h' -o -name '*.modulemap' -o -name 'go.mod' \) ! -name '*_test.go' \
  | LC_ALL=C sort | xargs shasum -a 256 | shasum -a 256 | cut -d' ' -f1

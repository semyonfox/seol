#!/bin/sh
set -eu

mkdir -p dist
version="${VERSION:-dev}"

build() {
  goos="$1"
  goarch="$2"
  asset_arch="$3"
  extension="${4:-}"
  goarm="${5:-}"
  echo "building ${goos}/${goarch}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOARM="$goarm" \
    go build -trimpath -ldflags="-s -w -X github.com/semyonfox/seol/internal/app.version=${version}" \
    -o "dist/seol_${goos}_${asset_arch}${extension}" ./cmd/seol
}

build linux amd64 x64
build linux arm64 arm64
build linux arm armv7 "" 7
build darwin amd64 x64
build darwin arm64 arm64
build windows amd64 x64 .exe
build windows arm64 arm64 .exe
build freebsd amd64 x64
build freebsd arm64 arm64

(cd dist &&
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum seol_*
  else
    shasum -a 256 seol_*
  fi > checksums.txt)

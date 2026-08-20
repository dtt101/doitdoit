#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <version-without-v> <GoReleaser-checksums-file> <output-directory>" >&2
  exit 2
fi

version=$1
checksums=$2
output=$3
template_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

if ! awk -v version="$version" 'BEGIN { exit(version ~ /^[0-9]+\.[0-9]+\.[0-9]+$/ ? 0 : 1) }'; then
  echo "version must be a stable numeric semver without a leading v" >&2
  exit 2
fi

x86_64_sha=$(awk '$2 == "doitdoit_Linux_x86_64.tar.gz" { print $1 }' "$checksums")
aarch64_sha=$(awk '$2 == "doitdoit_Linux_arm64.tar.gz" { print $1 }' "$checksums")
case "$x86_64_sha$aarch64_sha" in
  *[!0-9a-f]*) echo "release checksums contain non-hexadecimal characters" >&2; exit 1 ;;
esac
if [ "${#x86_64_sha}" -ne 64 ] || [ "${#aarch64_sha}" -ne 64 ]; then
  echo "could not find valid Linux x86_64 and arm64 SHA-256 hashes" >&2
  exit 1
fi

mkdir -p "$output"
sed \
  -e "s/@PKGVER@/$version/g" \
  -e "s/@X86_64_SHA256@/$x86_64_sha/g" \
  -e "s/@AARCH64_SHA256@/$aarch64_sha/g" \
  "$template_dir/PKGBUILD.in" > "$output/PKGBUILD"
sed \
  -e "s/@PKGVER@/$version/g" \
  -e "s/@X86_64_SHA256@/$x86_64_sha/g" \
  -e "s/@AARCH64_SHA256@/$aarch64_sha/g" \
  "$template_dir/SRCINFO.in" > "$output/.SRCINFO"
cp "$template_dir/LICENSE" "$output/LICENSE"

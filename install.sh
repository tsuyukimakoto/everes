#!/usr/bin/env bash
set -euo pipefail

repo="tsuyukimakoto/everes"
asset="everes-darwin-arm64"
bin_dir="${EVERES_BIN_DIR:-$HOME/bin}"
dest="$bin_dir/everes"

if [ "$#" -ne 1 ]; then
  echo "usage: install.sh vX.Y.Z" >&2
  exit 2
fi
version="$1"
if ! [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.+][0-9A-Za-z.-]+)?$ ]]; then
  echo "error: version must be a release tag such as v0.5.0" >&2
  exit 2
fi

if [ "$(uname -s)" != "Darwin" ] || [ "$(uname -m)" != "arm64" ]; then
  echo "error: supported platform is darwin/arm64" >&2
  exit 1
fi

release_url="https://github.com/${repo}/releases/download/${version}"
tmp_binary="$(mktemp)"
tmp_checksums="$(mktemp)"
cleanup() {
  rm -f -- "$tmp_binary" "$tmp_checksums"
}
trap cleanup EXIT HUP INT TERM

curl --proto '=https' --tlsv1.2 -fsSL --retry 3 \
  -o "$tmp_binary" "$release_url/$asset"
curl --proto '=https' --tlsv1.2 -fsSL --retry 3 \
  -o "$tmp_checksums" "$release_url/checksums.txt"

expected="$(awk -v asset="$asset" '$2 == asset || $2 == "*" asset { print $1 }' "$tmp_checksums")"
actual="$(shasum -a 256 "$tmp_binary" | awk '{ print $1 }')"
if [ -z "$expected" ] || [ "$actual" != "$expected" ]; then
  echo "error: checksum verification failed for $asset" >&2
  exit 1
fi

mkdir -p "$bin_dir"
install -m 0755 "$tmp_binary" "$dest"
echo "installed: $dest ($("$dest" version))"

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "warn: PATH に $bin_dir がありません。シェルの設定に追加してください" >&2 ;;
esac

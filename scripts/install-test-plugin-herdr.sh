#!/usr/bin/env bash
set -euo pipefail

# Digests from the official v0.7.0 release asset metadata.
case "$(uname -s)/$(uname -m)" in
  Linux/x86_64)
    asset=herdr-linux-x86_64
    sha256=ad2a5d480a4e04609a9dd30a19ec07854578df6b5f0ea9299246963baf40363b
    ;;
  Linux/aarch64|Linux/arm64)
    asset=herdr-linux-aarch64
    sha256=77407959c514c25c870bbcc6d2a2c86fef5b5701ed0c7c37745d7412e8563d72
    ;;
  Darwin/x86_64)
    asset=herdr-macos-x86_64
    sha256=6c61cdb67c79b8d0626e109b9d8d8635c66a80bfed21ac9fe6efdf1dd8d27c0f
    ;;
  Darwin/arm64)
    asset=herdr-macos-aarch64
    sha256=0946c1c5de396d1404906c81c84a0cef47af5e15c9aac3c058c3936b833fe311
    ;;
  *) echo 'Unsupported test platform' >&2; exit 1 ;;
esac

destination=bin/herdr-plugin-ci
mkdir -p bin
download=$(mktemp bin/.herdr-plugin-ci.XXXXXX)
trap 'rm -f "$download"' EXIT
curl --fail --location --silent --show-error --connect-timeout 10 --max-time 120 \
  "https://github.com/herdrdev/herdr/releases/download/v0.7.0/$asset" -o "$download"
printf '%s  %s\n' "$sha256" "$download" | shasum -a 256 -c -
chmod +x "$download"
test "$("$download" --version)" = 'herdr 0.7.0'
mv "$download" "$destination"
printf 'Installed %s (Herdr 0.7.0) at %s\n' "$asset" "$destination"

#!/usr/bin/env bash
set -euo pipefail

# Digests from the official v0.6.5 release asset metadata.
case "$(uname -s)/$(uname -m)" in
  Linux/x86_64)
    asset=herdr-linux-x86_64
    sha256=70ef4ce425c0697901a26b6c07562faf0d7f54d8c6b6df542a95a9774760e2bf
    ;;
  Linux/aarch64|Linux/arm64)
    asset=herdr-linux-aarch64
    sha256=78d5e27b335ae656218f2a23d355e9ccab0db32dcd85bec91945eb9acd7d8669
    ;;
  Darwin/x86_64)
    asset=herdr-macos-x86_64
    sha256=174ea85c099b1fbe8f217759959a7365a9a631d6d74d81b6a6bab642a7309444
    ;;
  Darwin/arm64)
    asset=herdr-macos-aarch64
    sha256=0938c67cc1c11762cf20ebc993be96d12c0fff784edc649c708d79ae8c67e8da
    ;;
  *) echo 'Unsupported test platform' >&2; exit 1 ;;
esac

destination=${1:-bin/herdr-ci}
mkdir -p "$(dirname "$destination")"
download=$(mktemp "$(dirname "$destination")/.herdr-ci.XXXXXX")
trap 'rm -f "$download"' EXIT
curl --fail --location --silent --show-error --connect-timeout 10 --max-time 120 \
  "https://github.com/herdrdev/herdr/releases/download/v0.6.5/$asset" -o "$download"
printf '%s  %s\n' "$sha256" "$download" | shasum -a 256 -c -
chmod +x "$download"
test "$("$download" --version)" = 'herdr 0.6.5'
mv "$download" "$destination"
printf 'Installed %s (Herdr 0.6.5) at %s\n' "$asset" "$destination"

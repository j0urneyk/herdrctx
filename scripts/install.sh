#!/bin/sh
set -eu

fail() {
    printf 'herdrctx install: %s\n' "$*" >&2
    exit 1
}

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
manifest="$root/herdr-plugin.toml"
[ -f "$manifest" ] || fail "Missing manifest: $manifest"
for tool in curl tar mktemp awk grep; do
    command -v "$tool" >/dev/null 2>&1 || fail "Required tool not found: $tool"
done

# Read only the top-level version in the repository's manifest format.
version=$(awk '
    /^[ \t]*\[/ { exit }
    /^[ \t]*version[ \t]*=/ {
        count++
        if ($0 !~ /^[ \t]*version[ \t]*=[ \t]*"[0-9A-Za-z.+-]+"[ \t]*(#.*)?$/) exit 1
        sub(/^[^"]*"/, "")
        sub(/".*$/, "")
        value = $0
    }
    END { if (count != 1 || value == "") exit 1; print value }
' "$manifest") || fail 'Expected one quoted top-level version in herdr-plugin.toml.'
printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$' \
    || fail "Invalid manifest version: $version"
printf '%s\n' "$version" | awk '
    /-/ {
        sub(/\+.*/, "")
        sub(/^[0-9]+\.[0-9]+\.[0-9]+-/, "")
        count = split($0, identifiers, ".")
        for (i = 1; i <= count; i++) if (identifiers[i] ~ /^0[0-9]+$/) exit 1
    }
' || fail "Invalid numeric prerelease identifier in version: $version"

case "$(uname -s)/$(uname -m)" in
    Darwin/x86_64) platform=macos_x86_64 ;;
    Darwin/arm64|Darwin/aarch64) platform=macos_aarch64 ;;
    Linux/x86_64) platform=linux_x86_64 ;;
    Linux/aarch64|Linux/arm64) platform=linux_aarch64 ;;
    *) fail 'Supported platforms: macOS/Linux on x86_64 or arm64.' ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
    checksum_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    checksum_tool=shasum
else
    fail 'Required tool not found: sha256sum or shasum.'
fi

install_dir=${HERDRCTX_INSTALL_DIR:-"$HOME/.local/bin"}
mkdir -p "$install_dir" || fail "Cannot create installation directory: $install_dir"
install_dir=$(CDPATH= cd -- "$install_dir" && pwd)
[ -w "$install_dir" ] || fail "Installation directory is not writable: $install_dir"
destination="$install_dir/herdrctx"
[ ! -d "$destination" ] || fail "Installation target is a directory: $destination"

temporary=$(mktemp -d)
staged=''
cleanup() {
    rm -rf "$temporary"
    [ -z "$staged" ] || rm -f "$staged"
}
trap cleanup 0
trap 'exit 130' INT
trap 'exit 143' TERM

asset="herdrctx_${version}_${platform}.tar.gz"
release_url="https://github.com/j0urneyk/herdrctx/releases/download/v$version"
for file in "$asset" checksums.txt; do
    curl --fail --location --silent --show-error --proto '=https' --proto-redir '=https' \
        --connect-timeout 10 --max-time 120 "$release_url/$file" -o "$temporary/$file" \
        || fail "Could not download $file for v$version. Check the published release assets."
done

expected=$(awk -v asset="$asset" '$2 == asset { count++; digest = $1 }
    END { if (count != 1) exit 1; print digest }' "$temporary/checksums.txt") \
    || fail "Expected one checksum for $asset."
printf '%s\n' "$expected" | grep -Eq '^[0-9a-fA-F]{64}$' || fail "Invalid SHA-256 for $asset."
if [ "$checksum_tool" = sha256sum ]; then
    actual=$(sha256sum "$temporary/$asset")
else
    actual=$(shasum -a 256 "$temporary/$asset")
fi
actual=${actual%% *}
[ "$actual" = "$expected" ] || fail "SHA-256 mismatch for $asset."

tar -xzf "$temporary/$asset" -C "$temporary" herdrctx || fail "Could not extract herdrctx from $asset."
[ -f "$temporary/herdrctx" ] && [ ! -L "$temporary/herdrctx" ] \
    || fail "Archive does not contain a regular herdrctx binary."
staged=$(mktemp "$install_dir/.herdrctx.XXXXXX") || fail "Cannot write to $install_dir."
cat "$temporary/herdrctx" > "$staged"
chmod 755 "$staged"
mv -f "$staged" "$destination" || fail "Cannot replace $destination."
staged=''

printf 'Installed herdrctx %s at %s\n' "$version" "$destination"
case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *) printf 'Add %s to PATH in your shell configuration.\n' "$install_dir" ;;
esac
selected=$(command -v herdrctx || true)
if [ -n "$selected" ] && [ "$selected" != "$destination" ]; then
    printf 'PATH currently selects %s. Check with: command -v herdrctx\n' "$selected"
fi
printf 'Run herdrctx from a terminal outside Herdr.\n'

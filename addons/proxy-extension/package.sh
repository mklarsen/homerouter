#!/usr/bin/env bash
set -Eeuo pipefail

SOURCE_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${1:?usage: package.sh OUTPUT_DIRECTORY}"

for command in node zip unzip sha256sum; do
    command -v "$command" >/dev/null 2>&1 || {
        echo "error: required command not found: $command" >&2
        exit 1
    }
done

VERSION="$(node -e 'const fs = require("node:fs"); process.stdout.write(JSON.parse(fs.readFileSync(process.argv[1], "utf8")).version);' "$SOURCE_DIR/manifest.json")"
[[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
    echo "error: invalid extension version: $VERSION" >&2
    exit 1
}

mkdir -p -- "$OUTPUT_DIR"
OUTPUT_DIR="$(cd -- "$OUTPUT_DIR" && pwd)"
ARCHIVE_NAME="homerouter-extension-${VERSION}.zip"
ARCHIVE_PATH="$OUTPUT_DIR/$ARCHIVE_NAME"
if [[ -e $ARCHIVE_PATH || -e $OUTPUT_DIR/SHA256SUMS ]]; then
    echo "error: output files already exist in $OUTPUT_DIR" >&2
    exit 1
fi

STAGING_DIR="$(mktemp -d)"
trap 'rm -rf -- "$STAGING_DIR"' EXIT
cp -- "$SOURCE_DIR/manifest.json" "$SOURCE_DIR/background.js" \
    "$SOURCE_DIR/popup.html" "$SOURCE_DIR/popup.js" \
    "$SOURCE_DIR/error.html" "$SOURCE_DIR/error.js" "$STAGING_DIR/"
cp -R -- "$SOURCE_DIR/icons" "$STAGING_DIR/icons"

(
    cd -- "$STAGING_DIR"
    zip -X -q -r "$ARCHIVE_PATH" \
        manifest.json background.js popup.html popup.js error.html error.js icons/
)

ENTRIES="$(unzip -Z1 "$ARCHIVE_PATH")"
for required in manifest.json background.js popup.html popup.js error.html error.js; do
    grep -Fqx -- "$required" <<< "$ENTRIES" || {
        echo "error: extension archive is missing $required" >&2
        exit 1
    }
done
for size in 16 32 48 128; do
    for variant in "icon${size}.png" "icon${size}-connected.png"; do
        grep -Fqx -- "icons/$variant" <<< "$ENTRIES" || {
            echo "error: extension archive is missing icons/$variant" >&2
            exit 1
        }
    done
done
if grep -Eq '(^|/)(README\.md|STORE_LISTING\.md|background\.test\.cjs)$' <<< "$ENTRIES"; then
    echo "error: development documentation or tests found in extension archive" >&2
    exit 1
fi

(
    cd -- "$OUTPUT_DIR"
    sha256sum "$ARCHIVE_NAME" > SHA256SUMS
)

printf 'Created %s\n' "$ARCHIVE_PATH"
cat "$OUTPUT_DIR/SHA256SUMS"

#!/usr/bin/env bash
#
# homerouter -- read CHANGELOG.md.
#
#   scripts/changelog.sh version         # newest released version, e.g. 0.2.0
#   scripts/changelog.sh notes [version] # that version's section body
#   scripts/changelog.sh check           # validate the file structure
#
# The newest version heading drives the release: merging a pull request that
# adds one publishes that release, merging anything else does not.
#
set -Eeuo pipefail

REPO_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
FILE="$REPO_DIR/CHANGELOG.md"
VERSION_RE='^## \[([0-9]+\.[0-9]+\.[0-9]+)\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$'

[[ -f $FILE ]] || { echo "error: CHANGELOG.md not found" >&2; exit 1; }

newest_version() {
    grep -oEm1 "$VERSION_RE" "$FILE" | sed -E 's/^## \[([^]]*)\].*/\1/'
}

section_body() {
    local want="$1"
    awk -v want="## [$want]" '
        index($0, want) == 1 { collecting = 1; next }
        collecting && /^## \[/ { exit }
        collecting { print }
    ' "$FILE" | sed -e '/./,$!d' -e :a -e '/^\n*$/{$d;N;};/\n$/ba'
}

case "${1:-version}" in
    version)
        v="$(newest_version)"
        [[ -n $v ]] || { echo "error: no '## [x.y.z] - YYYY-MM-DD' heading found" >&2; exit 1; }
        printf '%s\n' "$v"
        ;;
    notes)
        v="${2:-$(newest_version)}"
        body="$(section_body "$v")"
        [[ -n ${body//[[:space:]]/} ]] || { echo "error: section for $v is empty" >&2; exit 1; }
        printf '%s\n' "$body"
        ;;
    check)
        grep -qxF '## [Unreleased]' "$FILE" \
            || { echo "error: missing '## [Unreleased]' heading" >&2; exit 1; }
        v="$(newest_version)"
        [[ -n $v ]] || { echo "error: no valid version heading" >&2; exit 1; }
        section_body "$v" | grep -q '[^[:space:]]' \
            || { echo "error: section for $v is empty" >&2; exit 1; }
        # Every version heading must be well formed.
        while IFS= read -r line; do
            [[ $line == "## [Unreleased]" ]] && continue
            [[ $line =~ $VERSION_RE ]] \
                || { echo "error: malformed heading: $line" >&2; exit 1; }
        done < <(grep -E '^## \[' "$FILE")
        echo "CHANGELOG.md ok (newest version: $v)"
        ;;
    *)
        echo "usage: changelog.sh [version|notes [version]|check]" >&2
        exit 1
        ;;
esac

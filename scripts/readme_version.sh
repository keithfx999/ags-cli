#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/readme_version.sh validate <version>

Commands:
  validate    Validate literal README install example versions.

Notes:
  - <version> uses X.Y.Z format.
  - Literal examples may use X.Y.Z or vX.Y.Z.
  - Template references such as ${VERSION} are ignored.
EOF
}

validate_version_arg() {
  local version="$1"
  if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z]+(\.[0-9]+)?)?$ ]]; then
    echo "Invalid version: $version" >&2
    exit 1
  fi
}

validate_file() {
  local file="$1"
  local version="$2"
  local tag="v${version}"
  local bad

  if [ ! -f "$file" ]; then
    echo "Missing README file: $file" >&2
    exit 1
  fi

  bad="$(
    awk -v version="$version" -v tag="$tag" '
      {
        line = $0
        while (match(line, /VERSION[[:space:]]*=[[:space:]]*"?v?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z]+(\.[0-9]+)?)?"?/)) {
          prefix = substr(line, 1, RSTART - 1)
          if (substr(prefix, length(prefix), 1) == "$") {
            line = substr(line, RSTART + RLENGTH)
            continue
          }
          token = substr(line, RSTART, RLENGTH)
          value = token
          sub(/^VERSION[[:space:]]*=[[:space:]]*"?/, "", value)
          sub(/"?$/, "", value)
          if (value != version && value != tag) {
            printf("%d:%s\n", NR, token)
          }
          line = substr(line, RSTART + RLENGTH)
        }

        line = $0
        while (match(line, /\$VERSION[[:space:]]*=[[:space:]]*"?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z]+(\.[0-9]+)?)?"?/)) {
          token = substr(line, RSTART, RLENGTH)
          value = token
          sub(/^\$VERSION[[:space:]]*=[[:space:]]*"?/, "", value)
          sub(/"?$/, "", value)
          if (value != version) {
            printf("%d:%s\n", NR, token)
          }
          line = substr(line, RSTART + RLENGTH)
        }
      }
    ' "$file"
  )"

  if [ -n "$bad" ]; then
    echo "README install examples in $file must use version $version:" >&2
    printf '%s\n' "$bad" >&2
    exit 1
  fi
}

main() {
  if [ "$#" -lt 1 ]; then
    usage >&2
    exit 1
  fi

  case "$1" in
    validate)
      shift
      if [ "$#" -ne 1 ]; then
        usage >&2
        exit 1
      fi
      validate_version_arg "$1"
      validate_file README.md "$1"
      validate_file README-zh.md "$1"
      ;;
    *)
      usage >&2
      exit 1
      ;;
  esac
}

main "$@"

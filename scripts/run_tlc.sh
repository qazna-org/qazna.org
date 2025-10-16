#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <spec-path-without-extension>" >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SPEC_PATH="$1"
SPEC_DIR="$(cd "$ROOT_DIR" && dirname "$SPEC_PATH")"
SPEC_BASE="$(basename "$SPEC_PATH")"

TLA_VERSION="${TLA_VERSION:-1.7.3}"
CACHE_DIR="${XDG_CACHE_HOME:-${HOME}/.cache}/tlc"
mkdir -p "$CACHE_DIR"
JAR_PATH="${CACHE_DIR}/tla2tools-${TLA_VERSION}.jar"

if [ ! -f "$JAR_PATH" ]; then
  echo "Downloading TLA+ tools ${TLA_VERSION}..." >&2
  curl -fsSL -o "$JAR_PATH" \
    "https://github.com/tlaplus/tlaplus/releases/download/v${TLA_VERSION}/tla2tools.jar"
fi

cd "$ROOT_DIR/$SPEC_DIR"
java -cp "$JAR_PATH" tlc2.TLC -deadlock -cleanup "$SPEC_BASE"

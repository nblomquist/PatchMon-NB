#!/usr/bin/env bash
# Resolve the complete agent version without consulting server tags or NB_BUILD.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1-${AGENT_VERSION-$(cat "$ROOT/AGENT_VERSION")}}"
PATTERN='^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?(~nb[1-9][0-9]*)?$'
if [[ ! "$VERSION" =~ $PATTERN ]]; then
  echo "error: AGENT_VERSION must be a complete version, e.g. 2.1.6~nb3" >&2
  exit 1
fi
printf '%s\n' "$VERSION"

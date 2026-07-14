#!/bin/sh
set -eu

# verify-binary.sh — Verify chisel binary architecture and version
# Usage: verify-binary.sh <GOARCH> [<image:tag>] [<expected-version>]
#
# Checks:
#   1. Binary executes (docker run — catches exec format errors)
#   2. ELF Machine field matches expected GOARCH
#   3. Version string matches (if expected-version provided)

GOARCH="${1:?usage: verify-binary.sh GOARCH [image:tag] [expected-version]}"
IMAGE="${2:-chisel:test}"
EXPECTED_VER="${3:-}"

case "$GOARCH" in
  amd64)   EXPECTED_MACHINE="Advanced Micro Devices X86-64" ;;
  arm64)   EXPECTED_MACHINE="AArch64" ;;
  386)     EXPECTED_MACHINE="Intel 80386" ;;
  arm|armv5|armv6|armv7) EXPECTED_MACHINE="ARM" ;;
  ppc64le|ppc64) EXPECTED_MACHINE="PowerPC64" ;;
  mips|mipsle|mips64|mips64le) EXPECTED_MACHINE="MIPS" ;;
  s390x)   EXPECTED_MACHINE="IBM S/390" ;;
  *)       echo "error: unknown GOARCH: $GOARCH" >&2; exit 1 ;;
esac

echo "=== Verify $IMAGE (GOARCH=$GOARCH → $EXPECTED_MACHINE) ==="

# 1. Binary must execute
echo "--- chisel version ---"
docker run --rm "$IMAGE" chisel version

# 2. Extract and check ELF Machine field
echo "--- ELF architecture ---"
CID="$(docker create "$IMAGE")"
docker cp "$CID:/chisel" /tmp/chisel-verify-binary 2>/dev/null
docker rm "$CID" >/dev/null

MACHINE="$(readelf -h /tmp/chisel-verify-binary 2>/dev/null | sed -n 's/^[[:space:]]*Machine:[[:space:]]*//p')"
rm -f /tmp/chisel-verify-binary

if [ "$MACHINE" != "$EXPECTED_MACHINE" ]; then
  echo "FAIL: Machine mismatch (expected '$EXPECTED_MACHINE', got '$MACHINE')" >&2
  exit 1
fi
echo "  Machine: $MACHINE ✓"

# 3. Optional version check
if [ -n "$EXPECTED_VER" ]; then
  echo "--- version string ---"
  VER="$(docker run --rm "$IMAGE" chisel version 2>/dev/null | sed -n 's/^.*Version: *//p' | awk '{print $1}')"
  if [ "$VER" != "$EXPECTED_VER" ]; then
    echo "FAIL: Version mismatch (expected '$EXPECTED_VER', got '$VER')" >&2
    exit 1
  fi
  echo "  Version: $VER ✓"
fi

echo "PASS: $IMAGE"

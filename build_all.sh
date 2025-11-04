#!/usr/bin/env bash
set -euo pipefail

# build_all.sh - builds rustdns staticlib and then builds the Go binary linking to it.
# Usage: ./build_all.sh [target]
# target: optional Rust target triple for Raspberry Pi cross-build, e.g. armv7-unknown-linux-gnueabihf

TOPDIR=$(pwd)
RUSTDIR="$TOPDIR/rustdns"

# build rust for host first
echo "Building rustdns for host..."
cd "$RUSTDIR"
cargo build --release

# location of produced lib
LIBDIR="$RUSTDIR/target/release"
if [ ! -f "$LIBDIR/librustdns.a" ]; then
  echo "Warning: static lib not found at $LIBDIR/librustdns.a; linking may use dynamic lib instead"
fi

# build Go binary linking to rust lib
cd "$TOPDIR"
export CGO_ENABLED=1
export CGO_LDFLAGS="-L$LIBDIR -lrustdns"

echo "Building Go binary with CGO_LDFLAGS=$CGO_LDFLAGS"
go build -v -o piblock

echo "Built piblock (host)."

# Optional: cross-compile rust for target (if provided)
if [ "$#" -ge 1 ]; then
  TARGET="$1"
  echo "Cross-building rustdns for target $TARGET"
  rustup target add "$TARGET" || true
  cargo build --release --target "$TARGET"
  echo "Cross-build completed: $RUSTDIR/target/$TARGET/release"
  echo "To build Go binary for target you will need a C cross-compiler and set CC accordingly."
fi

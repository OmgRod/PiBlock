#!/usr/bin/env bash
set -euo pipefail

# install_and_run.sh
# Install system deps, build rustdns and the Go binary, register nginx site, and
# install/start a systemd service for PiBlock. Run this on the Raspberry Pi as root
# or via sudo from the repository root: `sudo ./deploy/install_and_run.sh`.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "PiBlock deploy script"
echo "Project root: $ROOT_DIR"

if [ "$EUID" -ne 0 ]; then
  echo "This script must be run as root (sudo)." >&2
  exit 1
fi

echo "Updating apt and installing packages..."
apt-get update -y
apt-get install -y nginx build-essential pkg-config ca-certificates curl git

echo "Ensuring OpenSSL dev is present (some crates need it)..."
apt-get install -y libssl-dev || true

echo "Building rustdns (release)..."
if [ -d "$ROOT_DIR/rustdns" ]; then
  pushd "$ROOT_DIR/rustdns" >/dev/null
  # If the user hasn't set up Rust, instruct
  if ! command -v cargo >/dev/null 2>&1; then
    echo "cargo not found. Please install Rust (rustup) first: https://rustup.rs" >&2
    popd >/dev/null
    exit 1
  fi
  cargo build --release
  popd >/dev/null
else
  echo "rustdns directory not found; skipping Rust build" >&2
fi

echo "Building Go binary (piblock)..."
if ! command -v go >/dev/null 2>&1; then
  echo "go not found. Please install Go on the Pi (apt or from golang.org)" >&2
  exit 1
fi
pushd "$ROOT_DIR" >/dev/null
# Build the main Go binary. If you want CGO linking to the rust staticlib, set
# CGO_ENABLED=1 and appropriate CGO_LDFLAGS before running this script.
go build -o piblock .
popd >/dev/null

NGINX_SITE_AVAILABLE=/etc/nginx/sites-available/piblock
NGINX_SITE_ENABLED=/etc/nginx/sites-enabled/piblock

echo "Installing nginx config..."
cp "$ROOT_DIR/deploy/nginx-piblock.conf" "$NGINX_SITE_AVAILABLE"
ln -sf "$NGINX_SITE_AVAILABLE" "$NGINX_SITE_ENABLED"

echo "Testing nginx config..."
nginx -t
systemctl restart nginx

SERVICE_PATH=/etc/systemd/system/piblock.service
echo "Installing systemd service to run '$ROOT_DIR/piblock'"
cat > "$SERVICE_PATH" <<EOF
[Unit]
Description=PiBlock DNS/Control Service
After=network.target

[Service]
Type=simple
WorkingDirectory=$ROOT_DIR
ExecStart=$ROOT_DIR/piblock
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now piblock.service

echo "Done. Services started."
echo "Check status: systemctl status piblock nginx"
echo "Check listening ports: ss -tuln | grep -E '3000|8081|8083|9080|53' || true"

exit 0

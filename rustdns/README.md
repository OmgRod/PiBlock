# rustdns - PiBlock Rust DNS server (control API + UDP resolver)

This is a small scaffold for a Rust-based DNS engine that will be controlled by the existing Go API.

Goals for this scaffold

- Expose an HTTP control API on `127.0.0.1:8082` with endpoints:
  - `POST /reload` — reload `./blocklist/*.txt` into memory
  - `GET /stats` — return query/blocked counters
- Run a UDP DNS resolver on `0.0.0.0:5353` (non-privileged port for testing). For production you can bind to port 53 with administrator privileges.
- For blocked domains (exact or simple wildcard `*.example.com`), reply `NXDOMAIN`. Otherwise forward to upstream DNS (default `1.1.1.1:53`).

How to run (dev)

1. Install Rust toolchain (https://rustup.rs)
2. From the repo root:

```sh
cd rustdns
cargo run --release
```

This will start the control HTTP API on `127.0.0.1:8082` and the UDP DNS server on `0.0.0.0:5353`.

Making DNS active for your local network

- Option A (recommended for testing): keep the DNS server on port 5353 and configure a single device to use `HOST_IP:5353` as its DNS server if the OS/device supports a custom port.
- Option B (bind to port 53): run as administrator (or set capabilities on Linux) to allow binding to port 53. Then set device DNS to the host IP.
- Option C (redirect): keep server on 5353 and use OS-level packet redirection (iptables on Linux / netsh on Windows) to forward port 53 traffic to port 5353.

Platform notes and helper commands

Windows (run PowerShell as Administrator)

- Start the server on port 5353 and forward port 53 to 5353 using netsh:

```powershell
# Run rustdns (PowerShell) - open an elevated PowerShell and run:
cd rustdns
cargo run --release

# In a separate elevated PowerShell, forward port 53 -> 127.0.0.1:5353
netsh interface portproxy add v4tov4 listenaddress=0.0.0.0 listenport=53 connectaddress=127.0.0.1 connectport=5353

# Verify:
netsh interface portproxy show all

# To remove the proxy later:
netsh interface portproxy delete v4tov4 listenaddress=0.0.0.0 listenport=53
```

Note: netsh portproxy requires the "IP Helper" service to be running. On Windows you can also run the Rust server directly binding to port 53 if you start PowerShell as Administrator and set `RUSTDNS_UDP_BIND=0.0.0.0:53` and `RUSTDNS_HTTP_ADDR=127.0.0.1:8082`.

Linux (iptables/nft)

- Recommended: keep server on 5353 and redirect system port 53 to 5353 with iptables (requires root):

```sh
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A PREROUTING -p udp --dport 53 -j REDIRECT --to-ports 5353
sudo iptables -t nat -A PREROUTING -p tcp --dport 53 -j REDIRECT --to-ports 5353

# Run server (as a normal user):
cd rustdns
cargo run --release

# To remove the rules:
sudo iptables -t nat -D PREROUTING -p udp --dport 53 -j REDIRECT --to-ports 5353
sudo iptables -t nat -D PREROUTING -p tcp --dport 53 -j REDIRECT --to-ports 5353
```

Systemd example (bind to 53 directly, requires CAP_NET_BIND_SERVICE):

```ini
[Unit]
Description=RustPiBlock DNS
After=network.target

[Service]
Environment=RUSTDNS_UDP_BIND=0.0.0.0:53
Environment=RUSTDNS_HTTP_ADDR=127.0.0.1:8082
ExecStart=/usr/bin/cargo run --release --manifest-path=/path/to/rustdns/Cargo.toml
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

When binding to port 53 directly, ensure the service runs with adequate privileges (either run as root or grant CAP_NET_BIND_SERVICE to the executable).

Next steps

- Integrate the Go API to POST `http://127.0.0.1:8082/reload` after list changes (done in the repository changes accompanying this scaffold).
- Improve wildcard matching semantics and add persistent metrics, TCP support, and optionally an HTTP streaming/event API.

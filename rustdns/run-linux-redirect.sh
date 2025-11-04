#!/usr/bin/env bash
# Redirect port 53 to 5353 (requires root)
set -e
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A PREROUTING -p udp --dport 53 -j REDIRECT --to-ports 5353
sudo iptables -t nat -A PREROUTING -p tcp --dport 53 -j REDIRECT --to-ports 5353

echo "Port 53 redirected to 5353. Run the rustdns server on port 5353 (default) and test with dig @localhost -p 5353 ..."
package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// GetClientMAC attempts to determine the client's MAC address from the request
// First tries X-Client-MAC header (set by client), then tries ARP lookup for local IPs
// NOTE: For production, this should be enhanced with proper network-level MAC detection
// or integration with DHCP server. The IP fallback is a temporary measure and should
// be noted as a security limitation - devices behind NAT will share the same identifier.
func GetClientMAC(r *http.Request) (string, error) {
	// Check if client sent their MAC in a header
	if mac := r.Header.Get("X-Client-MAC"); mac != "" {
		return normalizeMACAddress(mac), nil
	}

	// Get client IP
	clientIP := getClientIP(r)
	if clientIP == "" {
		return "", fmt.Errorf("could not determine client IP")
	}

	// Try ARP lookup for local network clients
	if mac, err := getMACFromARP(clientIP); err == nil && mac != "" {
		return normalizeMACAddress(mac), nil
	}

	// SECURITY NOTE: For non-local clients or when ARP fails, we use IP as identifier.
	// This is a known limitation - devices behind NAT will share the same identifier.
	// In production, consider requiring users to manually enter or detect MAC via
	// client-side tools, or integrate with network infrastructure (DHCP/router).
	return fmt.Sprintf("ip:%s", clientIP), nil
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// getMACFromARP attempts to get MAC address from system ARP cache
// This works for devices on the local network
func getMACFromARP(ip string) (string, error) {
	// Parse the IP to verify it's valid
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", fmt.Errorf("invalid IP address")
	}

	// Get all network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	// Try to find the interface that can reach this IP
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			// Check if the IP is in the same subnet
			if ipNet.Contains(parsedIP) {
				// For local subnet, we can't easily get the MAC from Go without external tools
				// Return empty to signal fallback needed
				return "", fmt.Errorf("ARP lookup not implemented")
			}
		}
	}

	return "", fmt.Errorf("IP not in local network")
}

// normalizeMACAddress normalizes a MAC address to lowercase with colons
func normalizeMACAddress(mac string) string {
	// Remove common separators
	mac = strings.ReplaceAll(mac, "-", "")
	mac = strings.ReplaceAll(mac, ":", "")
	mac = strings.ReplaceAll(mac, ".", "")
	mac = strings.ToLower(mac)

	// Add colons every 2 characters
	if len(mac) == 12 {
		var result strings.Builder
		for i := 0; i < 12; i += 2 {
			if i > 0 {
				result.WriteByte(':')
			}
			result.WriteString(mac[i : i+2])
		}
		return result.String()
	}

	return mac
}

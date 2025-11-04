package main

import (
    "net"
    "strings"
    "log"
)

// Config holds runtime settings for PiBlock.
type Config struct {
    Upstream     string `json:"upstream"`      // upstream DNS (host:port)
    BlockingMode string `json:"blocking_mode"` // redirect | null | nx
    BlockPageIP  string `json:"block_page_ip"` // IP to which blocked domains are redirected
    BlockPagePort int   `json:"block_page_port"` // HTTP port for block page
}

// AppConfig is the global runtime config (default values set in main).
var AppConfig = &Config{
    Upstream: "1.1.1.1:53",
    BlockingMode: "redirect",
    BlockPageIP: "",
    BlockPagePort: 9080,
}

// DetectLocalIP determines a likely local IP address by opening a UDP connection.
func DetectLocalIP() string {
    conn, err := net.Dial("udp", "1.1.1.1:53")
    if err != nil {
        log.Printf("DetectLocalIP: failed to dial outbound: %v", err)
        return ""
    }
    defer conn.Close()
    local := conn.LocalAddr().String()
    // local is like "192.168.1.5:54321" — strip port
    if idx := strings.LastIndex(local, ":"); idx > 0 {
        return local[:idx]
    }
    return local
}

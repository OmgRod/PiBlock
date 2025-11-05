package main

import (
	"log"
	"net"
	"strings"
	"sync"
)

// IPToMACCache caches IP to MAC address mappings
type IPToMACCache struct {
	mu      sync.RWMutex
	ipToMAC map[string]string // IP -> MAC
}

var ipMACCache = &IPToMACCache{
	ipToMAC: make(map[string]string),
}

// SetIPMAC stores an IP to MAC mapping
func (c *IPToMACCache) SetIPMAC(ip, mac string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ipToMAC[ip] = mac
	log.Printf("Cached IP %s -> MAC %s", ip, mac)
}

// GetMAC retrieves the MAC address for an IP
func (c *IPToMACCache) GetMAC(ip string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	mac, ok := c.ipToMAC[ip]
	return mac, ok
}

// IsBlockedForUser checks if a domain is blocked for a specific user
func (bm *BlocklistManager) IsBlockedForUser(domain, macAddress string, am *AccountManager) bool {
	if macAddress == "" {
		// No user identified, block nothing (or use default behavior)
		return false
	}

	// Get user's blocklists
	userLists, err := am.GetUserBlocklists(macAddress)
	if err != nil {
		log.Printf("Failed to get user blocklists for %s: %v", macAddress, err)
		return false
	}

	if len(userLists) == 0 {
		// User has no blocklists, nothing is blocked
		return false
	}

	// Check if domain matches any pattern in user's lists
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	for _, listName := range userLists {
		patterns, ok := bm.lists[listName]
		if !ok {
			continue
		}

		for _, pattern := range patterns {
			if pattern == "" {
				continue
			}
			// Check if it matches (using simple string matching or compile on-the-fly)
			re, err := patternToRegexp(pattern)
			if err == nil && re != nil && re.MatchString(d) {
				return true
			}
		}
	}

	return false
}

// GetClientIP extracts IP from address string
func GetClientIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

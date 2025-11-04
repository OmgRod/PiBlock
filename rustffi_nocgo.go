//go:build !cgo
// +build !cgo

package main

import "fmt"

// When cgo is disabled (common on some Pi setups or when CGO_ENABLED=0), provide
// a stub so the symbol exists and returns an error explaining what's needed.
func StartRustLinked(httpAddr, udpBind string) error {
    return fmt.Errorf("StartRustLinked unavailable: CGO is disabled. Rebuild with CGO_ENABLED=1 and link librustdns or use the subprocess fallback")
}

func StopRustLinked() error {
    return fmt.Errorf("StopRustLinked unavailable: CGO is disabled")
}

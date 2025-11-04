//go:build windows
package main

import "fmt"

// Stub implementations for Windows when cgo/static linking isn't available.
func StartRustLinked(httpAddr, udpBind string) error {
    return fmt.Errorf("StartRustLinked not supported on Windows in this build; use subprocess or build on Linux")
}

func StopRustLinked() error {
    return fmt.Errorf("StopRustLinked not supported on Windows in this build")
}

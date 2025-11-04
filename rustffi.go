//go:build !windows && cgo
package main

/*
#cgo linux LDFLAGS: -L${SRCDIR}/rustdns/target/release -lrustdns
#cgo darwin LDFLAGS: -L${SRCDIR}/rustdns/target/release -lrustdns
#cgo windows LDFLAGS: -L${SRCDIR}/rustdns/target/release -lrustdns
#include <stdlib.h>
#include "rustdns/include/rustdns.h"
*/
import "C"

import (
    "fmt"
    "unsafe"
)

// StartRustLinked starts the Rust DNS runtime linked via FFI.
func StartRustLinked(httpAddr, udpBind string) error {
    cHttp := C.CString(httpAddr)
    defer C.free(unsafe.Pointer(cHttp))
    cUdp := C.CString(udpBind)
    defer C.free(unsafe.Pointer(cUdp))
    rc := C.rustdns_start(cHttp, cUdp)
    if rc != 0 {
        return fmt.Errorf("rustdns_start returned %d", int(rc))
    }
    return nil
}

func StopRustLinked() error {
    rc := C.rustdns_stop()
    if rc != 0 {
        return fmt.Errorf("rustdns_stop returned %d", int(rc))
    }
    return nil
}

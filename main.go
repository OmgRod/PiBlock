package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
	"io"
)

func main() {
	// Initialize blocklist manager (loads ./blocklist/*.txt)
	bm, err := NewBlocklistManager("./blocklist")
	if err != nil {
		log.Fatalf("failed to initialize blocklist manager: %v", err)
	}

	// Start internal API server (binds to 127.0.0.1:8081)
	go func() {
		if err := StartInternalAPIServer(bm); err != nil {
			log.Fatalf("internal API server error: %v", err)
		}
	}()

	// Ensure block page server is running if redirect mode is enabled.
	if AppConfig.BlockingMode == "redirect" && AppConfig.BlockPagePort > 0 {
		// If no explicit BlockPageIP configured, attempt to detect a local IP reachable by clients
		if AppConfig.BlockPageIP == "" {
			if ip := DetectLocalIP(); ip != "" {
				AppConfig.BlockPageIP = ip
				log.Printf("detected local IP for block page: %s", ip)
			} else {
				log.Printf("could not detect local IP for block page; defaulting to 127.0.0.1")
				AppConfig.BlockPageIP = "127.0.0.1"
			}
		}
		StartBlockPageServer()
	}

	// Start DNS server: prefer calling into the Rust runtime via FFI (externs). If
	// that fails, fall back to launching a rust subprocess; if that also fails fall
	// back to the Go DNS server implementation.
	go func() {
		// Try to start linked rustdns via cgo FFI
		if err := StartRustLinked("127.0.0.1:9080", "0.0.0.0:5353"); err == nil {
			log.Printf("started rustdns via FFI")
			return
		} else {
			log.Printf("StartRustLinked failed: %v; trying subprocess approach", err)
		}

		// Try subprocess launch
		if err := startRustDNSIfPresent(); err != nil {
			log.Printf("rust dns subprocess start failed: %v; falling back to Go DNS server", err)
			if err2 := StartDNSServer(":53", bm); err2 != nil {
				log.Fatalf("DNS server error: %v", err2)
			}
		}
	}()

	// Try to auto-launch the Node frontend server (web/server.js).
	// Prefer a bundled Node runtime under ./node if present (so users don't need a global Node install).
	go func() {
		webDir := "./web"

		// determine bundled node/npm if available
		nodeDir := filepath.Join(".", "node")
		var nodeExe string
		var npmCmd string
		if runtime.GOOS == "windows" {
			nodeExe = filepath.Join(nodeDir, "node.exe")
			npmCmd = filepath.Join(nodeDir, "npm.cmd")
		} else {
			nodeExe = filepath.Join(nodeDir, "bin", "node")
			npmCmd = filepath.Join(nodeDir, "bin", "npm")
		}

		// helper to run npm (either bundled or system)
		runNpm := func(args ...string) error {
			// prefer bundled npm if it exists
			var cmd *exec.Cmd
			if _, err := os.Stat(npmCmd); err == nil {
				cmd = exec.Command(npmCmd, args...)
			} else {
				// fallback to system npm
				cmd = exec.Command("npm", args...)
			}
			cmd.Dir = webDir
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}

		// If dist folder missing, try to build the frontend. This will run bundled npm if present,
		// otherwise it requires npm available on PATH.
		distPath := filepath.Join(webDir, "dist")
		if _, err := os.Stat(distPath); os.IsNotExist(err) {
			log.Printf("web/dist not found; attempting to run npm ci && npm run build (bundled npm: %s)", npmCmd)
			// install deps
			if err := runNpm("ci"); err != nil {
				// fallback to npm install
				log.Printf("npm ci failed: %v; trying npm install", err)
				if err2 := runNpm("install"); err2 != nil {
					log.Printf("npm install also failed: %v", err2)
				}
			}
			// build
			if err := runNpm("run", "build"); err != nil {
				log.Printf("frontend build failed (ensure Node is available or bundled in ./node): %v", err)
			} else {
				log.Printf("frontend build completed")
			}
		}

		// Start the frontend in dev mode. Prefer a bundled npm if available and run
		// `npm run dev -- --host 0.0.0.0 --port 3000` so the dev server binds to all
		// interfaces and is reachable from other devices on the network.
		var startCmd *exec.Cmd
		if _, err := os.Stat(npmCmd); err == nil {
			startCmd = exec.Command(npmCmd, "run", "dev", "--", "--host", "0.0.0.0", "--port", "3000")
		} else {
			// fallback to system npm on PATH
			startCmd = exec.Command("npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "3000")
		}
		startCmd.Dir = webDir
		startCmd.Stdout = os.Stdout
		startCmd.Stderr = os.Stderr
		if err := startCmd.Start(); err != nil {
			log.Printf("failed to start frontend server: %v", err)
			return
		}
		log.Printf("started frontend process (pid=%d)", startCmd.Process.Pid)
		// don't wait here - let the process run independently
		// give it a moment to initialize
		time.Sleep(500 * time.Millisecond)
	}()

	fmt.Println("Frontend (Node) auto-launch attempted; public UI should be available if Node started")
	fmt.Println("DNS server started on :53 (udp)")

	// Block forever
	select {}
}

// startRustDNSIfPresent attempts to find a prebuilt Rust DNS binary and launch it as a
// subprocess. It sets sensible environment variables for the control API and UDP bind.
// If no binary is found it returns an error so the caller may fall back to Go DNS.
func startRustDNSIfPresent() error {
	// Search possible locations for a rustdns binary (developer builds and packaged paths)
	candidates := []string{
		"./rustdns/target/release/rustdns",
		"./rustdns/rustdns",
		"./rustdns/bin/rustdns",
		"./rustdns/bin/rustdns-linux",
	}
	var bin string
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			bin = c
			break
		}
	}
	// Also allow override from env
	if bin == "" {
		if p := os.Getenv("RUSTDNS_PATH"); p != "" {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				bin = p
			}
		}
	}
	if bin == "" {
		return fmt.Errorf("rustdns binary not found in known locations")
	}

	// Ensure executable bit on unix-like platforms
	if runtime.GOOS != "windows" {
		_ = os.Chmod(bin, 0755)
	}

	log.Printf("starting rustdns subprocess: %s", bin)
	cmd := exec.Command(bin)
	// configure rustdns control API and UDP bind via env
	env := os.Environ()
	// control API binds to localhost:9080 by default; make explicit
	env = append(env, "RUSTDNS_HTTP_ADDR=127.0.0.1:9080")
	// use non-privileged UDP port by default; system integrators can set RUSTDNS_UDP_BIND to :53
	env = append(env, "RUSTDNS_UDP_BIND=0.0.0.0:5353")
	cmd.Env = env
	// redirect stdout/stderr to our process logs
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}

	// stream subprocess logs
	go func() {
		io.Copy(os.Stdout, stdout)
	}()
	go func() {
		io.Copy(os.Stderr, stderr)
	}()

	// monitor process and return success (don't block) — if it exits soon, return error
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("rustdns exited: %v", err)
		} else {
			log.Printf("rustdns exited")
		}
	}()

	// return nil indicating we launched rustdns (caller may still choose to proceed)
	return nil
}

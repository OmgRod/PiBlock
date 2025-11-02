package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
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

	// Start DNS server (uses blocklist manager)
	go func() {
		if err := StartDNSServer(":53", bm); err != nil {
			log.Fatalf("DNS server error: %v", err)
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

		// Start the Node server. Prefer a bundled node.exe to run server.js if available.
		var startCmd *exec.Cmd
		serverJS := "server.js"
		if _, err := os.Stat(nodeExe); err == nil {
			startCmd = exec.Command(nodeExe, serverJS)
		} else {
			// fallback to `node server.js` via PATH
			startCmd = exec.Command("node", serverJS)
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

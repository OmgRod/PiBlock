package main

import (
    "log"
    "net/http"
    "strconv"
)

// StartBlockPageServer starts a minimal HTTP server serving a simple blocked page.
// It reads message content from AppConfig at request time, so toggling the mode
// affects the page without restarting (port changes require restart).
func StartBlockPageServer() {
    port := AppConfig.BlockPagePort
    mux := http.NewServeMux()
        mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "text/html; charset=utf-8")
                // Log some request details for diagnostics (don't log sensitive headers)
                ua := r.Header.Get("User-Agent")
                remote := r.RemoteAddr
                log.Printf("block page hit from %s UA=%s", remote, ua)

                // Serve a minimal, marginless responsive page. Keep it self-contained so it
                // displays correctly on very small screens.
                msg := `<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <title>Blocked by PiBlock</title>
    <style>
        /* reset margins so default browser stylesheet doesn't add space */
        html, body { margin: 0; padding: 0; height: 100%; }
        body { display:flex; align-items:center; justify-content:center; background:#071025; color:#e6eef6; font-family:Inter,system-ui,Segoe UI,Roboto,Helvetica,Arial; }
        .card { text-align:center; padding: 22px; max-width: 720px; box-sizing: border-box; }
        h1 { margin: 0 0 8px 0; font-size: 20px; }
        p { margin: 0 0 10px 0; font-size: 14px; color:#cfe3f6 }
        .meta { margin-top:10px; font-size:12px; color:#9fb6d9 }
        @media (max-width:480px) { h1 { font-size:18px } p { font-size:13px } }
    </style>
</head>
<body>
    <div class="card">
        <h1>Blocked by PiBlock DNS</h1>
        <p>This website has been blocked by your PiBlock DNS server.</p>
        <div class="meta">Request from: ` + remote + `</div>
        <div class="meta">User-Agent: ` + ua + `</div>
    </div>
</body>
</html>`

                _, _ = w.Write([]byte(msg))
        })

    addr := ":" + strconv.Itoa(port)
    srv := &http.Server{Addr: addr, Handler: mux}
    go func() {
        log.Printf("block page server listening on %s", addr)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Printf("block page server error: %v", err)
        }
    }()
}

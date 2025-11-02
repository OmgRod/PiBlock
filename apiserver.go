package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strconv"
    "net/url"
    "os"
    "path"
    "strings"
    "log"
    "errors"
)

// StartAPIServer starts a simple HTTP API to manage blocklist files.
// Endpoints:
// POST /lists/create    {"name":"listname","url":"https://..."}
// POST /lists/{name}/append    {"url":"https://..."}
// GET  /lists          returns existing lists and counts
// POST /reload         reloads all lists
// StartInternalAPIServer starts the internal-only API bound to localhost.
// This server is intended to be called by a public-facing Node/Express proxy
// or other trusted frontends. It binds to 127.0.0.1:8081 by default.
func StartInternalAPIServer(bm *BlocklistManager) error {
    addr := "127.0.0.1:8081"
    mux := http.NewServeMux()

    mux.HandleFunc("/lists/create", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
        log.Printf("API /lists/create %s %s", r.Method, r.URL.Path)
        var raw map[string]interface{}
        if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
            http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
            return
        }
        req := struct{ Name, URL string; Items []string }{}
        if v, ok := raw["name"].(string); ok { req.Name = v }
        if v, ok := raw["url"].(string); ok { req.URL = v }
        // items can be an array or a single string
        if it, ok := raw["items"]; ok {
            switch t := it.(type) {
            case string:
                // keep as single element, AddItemsToList will split
                req.Items = []string{t}
            case []interface{}:
                for _, e := range t {
                    if s, ok := e.(string); ok { req.Items = append(req.Items, s) }
                }
            }
        }
        // infer a list name from the URL if none provided
        if req.Name == "" && req.URL != "" {
            if u, err := url.Parse(req.URL); err == nil {
                base := path.Base(u.Path)
                // remove extension
                if ext := path.Ext(base); ext != "" {
                    base = strings.TrimSuffix(base, ext)
                }
                // fallback to host if base is empty
                if base == "" {
                    base = u.Hostname()
                }
                req.Name = base
            }
        }
        // Require name and either url or items
        if req.Name == "" || (req.URL == "" && len(req.Items) == 0) {
            log.Printf("API /lists/create missing name/url/items after inference: name=%q url=%q items=%d", req.Name, req.URL, len(req.Items))
            http.Error(w, "missing list name or url/items", http.StatusBadRequest)
            return
        }
        var added int
        var err error
        if req.URL != "" {
            added, err = bm.AddFileToList(req.Name, req.URL, true)
        } else {
            added, err = bm.AddItemsToList(req.Name, req.Items, true)
        }
        if err != nil {
            log.Printf("API /lists/create error: %v", err)
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        log.Printf("API /lists/create wrote %d lines to %s", added, req.Name)
        fmt.Fprintf(w, "added %d lines to %s\n", added, req.Name)
    })

    // GET /lists/items/{name}?offset=0&limit=100&q=foo
    // DELETE /lists/items/{name}  with JSON body {"domain":"example.com"}
    mux.HandleFunc("/lists/items/", func(w http.ResponseWriter, r *http.Request) {
        // path after prefix
        listName := strings.TrimPrefix(r.URL.Path, "/lists/items/")
        if listName == "" {
            http.Error(w, "missing list name", http.StatusBadRequest)
            return
        }
        switch r.Method {
        case http.MethodGet:
            q := r.URL.Query().Get("q")
            offStr := r.URL.Query().Get("offset")
            limStr := r.URL.Query().Get("limit")
            offset := 0
            limit := 100
            if offStr != "" {
                if v, err := strconv.Atoi(offStr); err == nil { offset = v }
            }
            if limStr != "" {
                if v, err := strconv.Atoi(limStr); err == nil { limit = v }
            }
            total, items, err := bm.ListDomains(listName, offset, limit, q)
            if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                    http.Error(w, "list not found", http.StatusNotFound)
                    return
                }
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            resp := map[string]interface{}{"total": total, "items": items, "offset": offset, "limit": limit}
            json.NewEncoder(w).Encode(resp)
            return
        case http.MethodDelete:
            var req struct{ Domain string `json:"domain"` }
            if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                http.Error(w, "invalid json", http.StatusBadRequest)
                return
            }
            if req.Domain == "" {
                http.Error(w, "missing domain", http.StatusBadRequest)
                return
            }
            removed, err := bm.RemoveDomain(listName, req.Domain)
            if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                    http.Error(w, "list not found", http.StatusNotFound)
                    return
                }
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            if !removed {
                http.Error(w, "domain not found", http.StatusNotFound)
                return
            }
            w.WriteHeader(http.StatusOK)
            json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
            return
        default:
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
    })

    mux.HandleFunc("/lists/", func(w http.ResponseWriter, r *http.Request) {
        // handle /lists/{name}/append
        p := strings.TrimPrefix(r.URL.Path, "/lists/")
        if p == "" {
            // list lists (use in-memory lists counts to avoid mismatch between file newline counts and parsed entries)
            if r.Method != http.MethodGet {
                http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
                return
            }
            lists := make(map[string]int)
            bm.mu.RLock()
            for name, arr := range bm.lists {
                lists[name] = len(arr)
            }
            bm.mu.RUnlock()
            _ = json.NewEncoder(w).Encode(lists)
            return
        }

        // expecting {name}/append or {name}/delete or {name}/download or {name}/replace
        parts := strings.SplitN(p, "/", 2)
        name := parts[0]
        if len(parts) == 2 && parts[1] == "append" {
            if r.Method != http.MethodPost {
                http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
                return
            }
            var raw map[string]interface{}
            if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
                http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
                return
            }
            // allow {"url":"..."} or {"items":"a,b,c"} or {"items":["a","b"]}
            if v, ok := raw["url"].(string); ok && v != "" {
                added, err := bm.AddFileToList(name, v, false)
                if err != nil {
                    log.Printf("API /lists/%s/append error: %v", name, err)
                    http.Error(w, err.Error(), http.StatusInternalServerError)
                    return
                }
                log.Printf("API /lists/%s/append added %d lines", name, added)
                fmt.Fprintf(w, "added %d lines to %s\n", added, name)
                return
            }
            // items
            var items []string
            if it, ok := raw["items"]; ok {
                switch t := it.(type) {
                case string:
                    items = []string{t}
                case []interface{}:
                    for _, e := range t {
                        if s, ok := e.(string); ok { items = append(items, s) }
                    }
                }
            }
            if len(items) == 0 {
                http.Error(w, "missing url or items", http.StatusBadRequest)
                return
            }
            added, err := bm.AddItemsToList(name, items, false)
            if err != nil {
                log.Printf("API /lists/%s/append error: %v", name, err)
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            log.Printf("API /lists/%s/append added %d lines", name, added)
            fmt.Fprintf(w, "added %d lines to %s\n", added, name)
            return
        }

        if len(parts) == 2 && parts[1] == "delete" {
            if r.Method != http.MethodDelete {
                http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
                return
            }
            fp := path.Join(bm.dir, name+".txt")
            if err := os.Remove(fp); err != nil {
                    log.Printf("API delete %s error: %v", fp, err)
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            _ = bm.LoadAll()
                log.Printf("API deleted list %s", name)
            io.WriteString(w, "deleted\n")
            return
        }

        if len(parts) == 2 && parts[1] == "replace" {
            if r.Method != http.MethodPost {
                http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
                return
            }
            var req struct{ URL string `json:"url"` }
            if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                    log.Printf("API replace bad request: %v", err)
                http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
                return
            }
            written, err := bm.ReplaceListFromURL(name, req.URL)
            if err != nil {
                    log.Printf("API replace error: %v", err)
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
                log.Printf("API replace wrote %d lines to %s", written, name)
            fmt.Fprintf(w, "wrote %d lines to %s\n", written, name)
            return
        }

        http.NotFound(w, r)
    })

    mux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
        if err := bm.LoadAll(); err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        io.WriteString(w, "reloaded\n")
    })

    // validate remote file (do not save)
    mux.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
        var req struct{ URL string `json:"url"` }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
            return
        }
        client := &http.Client{}
        resp, err := client.Get(req.URL)
        if err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        defer resp.Body.Close()
        if resp.StatusCode < 200 || resp.StatusCode >= 300 {
            http.Error(w, "fetch failed: "+resp.Status, http.StatusBadRequest)
            return
        }
        lines, err := readLines(resp.Body)
        if err != nil {
            http.Error(w, "parse error: "+err.Error(), http.StatusBadRequest)
            return
        }
        // return number of parsed domains and a small sample
        sample := []string{}
        for i := 0; i < len(lines) && i < 10; i++ { sample = append(sample, lines[i]) }
        out := map[string]interface{}{"count": len(lines), "sample": sample}
        _ = json.NewEncoder(w).Encode(out)
    })

    // analytics
    mux.HandleFunc("/analytics", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
        s := bm.GetStats()
        _ = json.NewEncoder(w).Encode(s)
    })

    // recent logs - GET returns recent entries; DELETE clears logs
    mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodGet:
            q := r.URL.Query()
            limit := 100
            if v := q.Get("limit"); v != "" {
                if n, err := strconv.Atoi(v); err == nil {
                    limit = n
                }
            }
            logs := bm.GetLogs(limit)
            _ = json.NewEncoder(w).Encode(logs)
            return
        case http.MethodDelete:
            if err := bm.DeleteLogs(); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            _ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
            return
        default:
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
    })

    return http.ListenAndServe(addr, mux)
}

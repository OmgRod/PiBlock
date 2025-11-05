package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// handleListCreate handles list creation with per-user filtering
func handleListCreate(w http.ResponseWriter, r *http.Request, bm *BlocklistManager, am *AccountManager) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	isGuest := r.Header.Get("X-Is-Guest") == "true"
	if isGuest {
		http.Error(w, "guests cannot create lists", http.StatusForbidden)
		return
	}

	userMAC := r.Header.Get("X-User-MAC")

	log.Printf("API /lists/create %s %s (user: %s)", r.Method, r.URL.Path, userMAC)
	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	req := struct{ Name, URL string; Items []string }{}
	if v, ok := raw["name"].(string); ok {
		req.Name = v
	}
	if v, ok := raw["url"].(string); ok {
		req.URL = v
	}
	if it, ok := raw["items"]; ok {
		switch t := it.(type) {
		case string:
			req.Items = []string{t}
		case []interface{}:
			for _, e := range t {
				if s, ok := e.(string); ok {
					req.Items = append(req.Items, s)
				}
			}
		}
	}

	// Infer list name from URL if not provided
	if req.Name == "" && req.URL != "" {
		if u, err := url.Parse(req.URL); err == nil {
			base := path.Base(u.Path)
			if ext := path.Ext(base); ext != "" {
				base = strings.TrimSuffix(base, ext)
			}
			if base == "" {
				base = u.Hostname()
			}
			req.Name = base
		}
	}

	// Require name and either url or items
	if req.Name == "" || (req.URL == "" && len(req.Items) == 0) {
		log.Printf("API /lists/create missing name/url/items: name=%q url=%q items=%d", req.Name, req.URL, len(req.Items))
		http.Error(w, "missing list name or url/items", http.StatusBadRequest)
		return
	}

	// Prefix list name with user's MAC to make it per-user
	userListName := fmt.Sprintf("%s_%s", userMAC, req.Name)

	var added int
	var err error
	if req.URL != "" {
		added, err = bm.AddFileToList(userListName, req.URL, true)
	} else {
		added, err = bm.AddItemsToList(userListName, req.Items, true)
	}
	if err != nil {
		log.Printf("API /lists/create error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Associate list with user
	if err := am.AddUserBlocklist(userMAC, userListName); err != nil {
		log.Printf("Failed to associate list with user: %v", err)
	}

	log.Printf("API /lists/create wrote %d lines to %s for user %s", added, userListName, userMAC)
	fmt.Fprintf(w, "added %d lines to %s\n", added, req.Name)
	go notifyRustReload()
}

// handleListItems handles getting/deleting items from a list
func handleListItems(w http.ResponseWriter, r *http.Request, bm *BlocklistManager, am *AccountManager) {
	listName := strings.TrimPrefix(r.URL.Path, "/lists/items/")
	if listName == "" {
		http.Error(w, "missing list name", http.StatusBadRequest)
		return
	}

	userMAC := r.Header.Get("X-User-MAC")
	isGuest := r.Header.Get("X-Is-Guest") == "true"

	// Prefix with user MAC to ensure they can only access their lists
	userListName := fmt.Sprintf("%s_%s", userMAC, listName)

	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query().Get("q")
		offStr := r.URL.Query().Get("offset")
		limStr := r.URL.Query().Get("limit")
		offset := 0
		limit := 100
		if offStr != "" {
			if v, err := strconv.Atoi(offStr); err == nil {
				offset = v
			}
		}
		if limStr != "" {
			if v, err := strconv.Atoi(limStr); err == nil {
				limit = v
			}
		}
		total, items, err := bm.ListDomains(userListName, offset, limit, q)
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
		if isGuest {
			http.Error(w, "guests cannot delete items", http.StatusForbidden)
			return
		}

		var req struct{ Domain string `json:"domain"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Domain == "" {
			http.Error(w, "missing domain", http.StatusBadRequest)
			return
		}
		removed, err := bm.RemoveDomain(userListName, req.Domain)
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
}

// handleLists handles listing and managing lists
func handleLists(w http.ResponseWriter, r *http.Request, bm *BlocklistManager, am *AccountManager) {
	p := strings.TrimPrefix(r.URL.Path, "/lists/")
	userMAC := r.Header.Get("X-User-MAC")
	isGuest := r.Header.Get("X-Is-Guest") == "true"

	if p == "" {
		// List user's lists only
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get user's blocklists
		userLists, err := am.GetUserBlocklists(userMAC)
		if err != nil {
			log.Printf("Failed to get user blocklists: %v", err)
			userLists = []string{}
		}

		lists := make(map[string]int)
		bm.mu.RLock()
		for _, fullName := range userLists {
			if arr, ok := bm.lists[fullName]; ok {
				// Strip user prefix for display
				displayName := strings.TrimPrefix(fullName, userMAC+"_")
				lists[displayName] = len(arr)
			}
		}
		bm.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(lists)
		return
	}

	// Handle specific list operations
	parts := strings.SplitN(p, "/", 2)
	name := parts[0]
	userListName := fmt.Sprintf("%s_%s", userMAC, name)

	if len(parts) == 2 && parts[1] == "append" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if isGuest {
			http.Error(w, "guests cannot append", http.StatusForbidden)
			return
		}

		var raw map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		if v, ok := raw["url"].(string); ok && v != "" {
			added, err := bm.AddFileToList(userListName, v, false)
			if err != nil {
				log.Printf("API /lists/%s/append error: %v", name, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			log.Printf("API /lists/%s/append added %d lines", name, added)
			fmt.Fprintf(w, "added %d lines to %s\n", added, name)
			go notifyRustReload()
			return
		}

		var items []string
		if it, ok := raw["items"]; ok {
			switch t := it.(type) {
			case string:
				items = []string{t}
			case []interface{}:
				for _, e := range t {
					if s, ok := e.(string); ok {
						items = append(items, s)
					}
				}
			}
		}
		if len(items) == 0 {
			http.Error(w, "missing url or items", http.StatusBadRequest)
			return
		}
		added, err := bm.AddItemsToList(userListName, items, false)
		if err != nil {
			log.Printf("API /lists/%s/append error: %v", name, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("API /lists/%s/append added %d lines", name, added)
		fmt.Fprintf(w, "added %d lines to %s\n", added, name)
		go notifyRustReload()
		return
	}

	if len(parts) == 2 && parts[1] == "delete" {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if isGuest {
			http.Error(w, "guests cannot delete", http.StatusForbidden)
			return
		}

		fp := path.Join(bm.dir, userListName+".txt")
		if err := os.Remove(fp); err != nil {
			log.Printf("API delete %s error: %v", fp, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Remove from user's blocklist associations
		if err := am.RemoveUserBlocklist(userMAC, userListName); err != nil {
			log.Printf("Failed to remove user blocklist association: %v", err)
		}
		
		_ = bm.LoadAll()
		log.Printf("API deleted list %s for user %s", name, userMAC)
		io.WriteString(w, "deleted\n")
		go notifyRustReload()
		return
	}

	if len(parts) == 2 && parts[1] == "replace" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if isGuest {
			http.Error(w, "guests cannot replace", http.StatusForbidden)
			return
		}

		var req struct{ URL string `json:"url"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("API replace bad request: %v", err)
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		written, err := bm.ReplaceListFromURL(userListName, req.URL)
		if err != nil {
			log.Printf("API replace error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("API replace wrote %d lines to %s for user %s", written, name, userMAC)
		fmt.Fprintf(w, "wrote %d lines to %s\n", written, name)
		go notifyRustReload()
		return
	}

	http.NotFound(w, r)
}

// handleLogs handles log operations
func handleLogs(w http.ResponseWriter, r *http.Request, bm *BlocklistManager, am *AccountManager) {
	isGuest := r.Header.Get("X-Is-Guest") == "true"

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
		if isGuest {
			http.Error(w, "guests cannot delete logs", http.StatusForbidden)
			return
		}
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
}

// handleValidate validates a remote blocklist URL
func handleValidate(bm *BlocklistManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct{ URL string `json:"url"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		client := &http.Client{Timeout: 15 * time.Second}
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
		sample := []string{}
		for i := 0; i < len(lines) && i < 10; i++ {
			sample = append(sample, lines[i])
		}
		out := map[string]interface{}{"count": len(lines), "sample": sample}
		_ = json.NewEncoder(w).Encode(out)
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"log"
)

// StartAuthAPIServer starts API endpoints for account management
func StartAuthAPIServer(am *AccountManager, addr string) error {
	mux := http.NewServeMux()

	// Account setup/check endpoint
	mux.HandleFunc("/auth/check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			MACAddress string `json:"mac_address"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if req.MACAddress == "" {
			// Try to detect MAC from request
			mac, err := GetClientMAC(r)
			if err != nil {
				http.Error(w, "could not determine MAC address", http.StatusBadRequest)
				return
			}
			req.MACAddress = mac
		}

		exists, err := am.AccountExists(req.MACAddress)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		resp := map[string]interface{}{
			"exists":      exists,
			"mac_address": req.MACAddress,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Create account
	mux.HandleFunc("/auth/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			MACAddress string `json:"mac_address"`
			Passcode   string `json:"passcode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if req.MACAddress == "" {
			mac, err := GetClientMAC(r)
			if err != nil {
				http.Error(w, "could not determine MAC address", http.StatusBadRequest)
				return
			}
			req.MACAddress = mac
		}

		if req.Passcode == "" {
			http.Error(w, "passcode is required", http.StatusBadRequest)
			return
		}

		if err := am.CreateAccount(req.MACAddress, req.Passcode); err != nil {
			log.Printf("Failed to create account: %v", err)
			http.Error(w, fmt.Sprintf("failed to create account: %v", err), http.StatusInternalServerError)
			return
		}

		// Create session after account creation
		session := am.createSession(req.MACAddress, false)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    true,
			"session_id": session.ID,
			"message":    "Account created successfully",
		})
	})

	// Login
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			MACAddress string `json:"mac_address"`
			Passcode   string `json:"passcode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if req.MACAddress == "" {
			mac, err := GetClientMAC(r)
			if err != nil {
				http.Error(w, "could not determine MAC address", http.StatusBadRequest)
				return
			}
			req.MACAddress = mac
		}

		session, err := am.Authenticate(req.MACAddress, req.Passcode)
		if err != nil {
			log.Printf("Authentication failed for MAC %s: %v", req.MACAddress, err)
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    true,
			"session_id": session.ID,
			"is_guest":   session.IsGuest,
		})
	})

	// Guest login
	mux.HandleFunc("/auth/guest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			MACAddress string `json:"mac_address"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if req.MACAddress == "" {
			mac, err := GetClientMAC(r)
			if err != nil {
				http.Error(w, "could not determine MAC address", http.StatusBadRequest)
				return
			}
			req.MACAddress = mac
		}

		session := am.CreateGuestSession(req.MACAddress)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    true,
			"session_id": session.ID,
			"is_guest":   true,
		})
	})

	// Logout
	mux.HandleFunc("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		am.InvalidateSession(req.SessionID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})
	})

	// Verify session
	mux.HandleFunc("/auth/verify", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		session, err := am.GetSession(req.SessionID)
		if err != nil {
			http.Error(w, "invalid session", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":       true,
			"is_guest":    session.IsGuest,
			"mac_address": session.MACAddress,
		})
	})

	// Change passcode
	mux.HandleFunc("/auth/change-passcode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionID   string `json:"session_id"`
			OldPasscode string `json:"old_passcode"`
			NewPasscode string `json:"new_passcode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		session, err := am.GetSession(req.SessionID)
		if err != nil {
			http.Error(w, "invalid session", http.StatusUnauthorized)
			return
		}

		if session.IsGuest {
			http.Error(w, "guests cannot change passcode", http.StatusForbidden)
			return
		}

		if err := am.ChangePasscode(session.MACAddress, req.OldPasscode, req.NewPasscode); err != nil {
			log.Printf("Failed to change passcode: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Passcode changed successfully",
		})
	})

	log.Printf("Auth API server starting on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// authMiddleware checks for valid session and adds user info to request context
func authMiddleware(am *AccountManager, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get session ID from header
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID == "" {
			http.Error(w, "missing session", http.StatusUnauthorized)
			return
		}

		session, err := am.GetSession(sessionID)
		if err != nil {
			http.Error(w, "invalid or expired session", http.StatusUnauthorized)
			return
		}

		// Add session info to headers for downstream handlers
		r.Header.Set("X-User-MAC", session.MACAddress)
		r.Header.Set("X-Is-Guest", fmt.Sprintf("%t", session.IsGuest))

		next(w, r)
	}
}

// guestAllowedMiddleware checks session and allows guests for read-only operations
func guestAllowedMiddleware(am *AccountManager, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID == "" {
			http.Error(w, "missing session", http.StatusUnauthorized)
			return
		}

		session, err := am.GetSession(sessionID)
		if err != nil {
			http.Error(w, "invalid or expired session", http.StatusUnauthorized)
			return
		}

		// Check if guest is trying to modify
		if session.IsGuest && r.Method != http.MethodGet {
			http.Error(w, "guests can only view, not modify", http.StatusForbidden)
			return
		}

		r.Header.Set("X-User-MAC", session.MACAddress)
		r.Header.Set("X-Is-Guest", fmt.Sprintf("%t", session.IsGuest))

		next(w, r)
	}
}

// StartInternalAPIServerWithAuth starts the internal API with authentication
func StartInternalAPIServerWithAuth(bm *BlocklistManager, am *AccountManager) error {
	addr := "127.0.0.1:8081"
	mux := http.NewServeMux()

	// Wrap handlers with authentication middleware
	// For lists operations, use guestAllowedMiddleware to allow read-only guest access
	
	// Lists endpoints - guests can view
	mux.HandleFunc("/lists/create", guestAllowedMiddleware(am, func(w http.ResponseWriter, r *http.Request) {
		handleListCreate(w, r, bm, am)
	}))

	mux.HandleFunc("/lists/items/", guestAllowedMiddleware(am, func(w http.ResponseWriter, r *http.Request) {
		handleListItems(w, r, bm, am)
	}))

	mux.HandleFunc("/lists/", guestAllowedMiddleware(am, func(w http.ResponseWriter, r *http.Request) {
		handleLists(w, r, bm, am)
	}))

	// Analytics - guests can view
	mux.HandleFunc("/analytics", guestAllowedMiddleware(am, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		// Filter analytics by user's MAC address if not guest
		isGuest := r.Header.Get("X-Is-Guest") == "true"
		userMAC := r.Header.Get("X-User-MAC")
		
		s := bm.GetStats()
		
		// If not a guest, could filter stats by user - for now return all
		// In a production system, you'd track per-user analytics
		if !isGuest && userMAC != "" {
			// Future: filter by user
		}
		
		_ = json.NewEncoder(w).Encode(s)
	}))

	// Logs - guests can view
	mux.HandleFunc("/logs", guestAllowedMiddleware(am, func(w http.ResponseWriter, r *http.Request) {
		handleLogs(w, r, bm, am)
	}))

	// Reload - authenticated users only
	mux.HandleFunc("/reload", authMiddleware(am, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		isGuest := r.Header.Get("X-Is-Guest") == "true"
		if isGuest {
			http.Error(w, "guests cannot reload", http.StatusForbidden)
			return
		}
		
		if err := bm.LoadAll(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		io.WriteString(w, "reloaded\n")
	}))

	// Validate - no auth required
	mux.HandleFunc("/validate", handleValidate(bm))

	log.Printf("Internal API server with auth starting on %s", addr)
	return http.ListenAndServe(addr, mux)
}

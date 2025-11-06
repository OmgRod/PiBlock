package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// AccountManager handles user accounts based on MAC addresses
type AccountManager struct {
	db       *sql.DB
	mu       sync.RWMutex
	sessions map[string]*Session // sessionID -> Session
}

// Account represents a user account identified by MAC address
type Account struct {
	ID           int64
	MACAddress   string
	PasscodeHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session represents an active user session
type Session struct {
	ID         string
	MACAddress string
	IsGuest    bool
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// NewAccountManager initializes the account database and manager
func NewAccountManager(dataDir string) (*AccountManager, error) {
	// Ensure the data directory exists so SQLite can create the DB file
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to ensure data dir %s: %w", dataDir, err)
	}

	dbPath := filepath.Join(dataDir, "accounts.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open accounts database: %w", err)
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mac_address TEXT UNIQUE NOT NULL,
		passcode_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE INDEX IF NOT EXISTS idx_mac_address ON accounts(mac_address);
	
	CREATE TABLE IF NOT EXISTS user_blocklists (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mac_address TEXT NOT NULL,
		list_name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(mac_address, list_name),
		FOREIGN KEY (mac_address) REFERENCES accounts(mac_address) ON DELETE CASCADE
	);
	
	CREATE INDEX IF NOT EXISTS idx_user_blocklists_mac ON user_blocklists(mac_address);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	am := &AccountManager{
		db:       db,
		sessions: make(map[string]*Session),
	}

	// Clean up expired sessions periodically
	go am.cleanupSessions()

	log.Printf("AccountManager initialized with database at %s", dbPath)
	return am, nil
}

// Close closes the database connection
func (am *AccountManager) Close() error {
	return am.db.Close()
}

// CreateAccount creates a new account for a MAC address with a passcode
func (am *AccountManager) CreateAccount(macAddress, passcode string) error {
	if macAddress == "" || passcode == "" {
		return errors.New("MAC address and passcode are required")
	}

	// Hash the passcode
	hash, err := bcrypt.GenerateFromPassword([]byte(passcode), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash passcode: %w", err)
	}

	// Insert into database
	_, err = am.db.Exec(
		"INSERT INTO accounts (mac_address, passcode_hash) VALUES (?, ?)",
		macAddress, string(hash),
	)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	log.Printf("Created account for MAC: %s", macAddress)
	return nil
}

// Authenticate verifies a MAC address and passcode, returns a session
func (am *AccountManager) Authenticate(macAddress, passcode string) (*Session, error) {
	if macAddress == "" || passcode == "" {
		return nil, errors.New("MAC address and passcode are required")
	}

	var passcodeHash string
	err := am.db.QueryRow(
		"SELECT passcode_hash FROM accounts WHERE mac_address = ?",
		macAddress,
	).Scan(&passcodeHash)

	if err == sql.ErrNoRows {
		return nil, errors.New("account not found")
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Verify passcode
	if err := bcrypt.CompareHashAndPassword([]byte(passcodeHash), []byte(passcode)); err != nil {
		return nil, errors.New("invalid passcode")
	}

	// Create session
	session := am.createSession(macAddress, false)
	log.Printf("Authenticated user with MAC: %s", macAddress)
	return session, nil
}

// CreateGuestSession creates a guest session for viewing only
func (am *AccountManager) CreateGuestSession(macAddress string) *Session {
	session := am.createSession(macAddress, true)
	log.Printf("Created guest session for MAC: %s", macAddress)
	return session
}

// createSession creates and stores a new session
func (am *AccountManager) createSession(macAddress string, isGuest bool) *Session {
	sessionID := generateSessionID()
	session := &Session{
		ID:         sessionID,
		MACAddress: macAddress,
		IsGuest:    isGuest,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(24 * time.Hour), // 24 hour sessions
	}

	am.mu.Lock()
	am.sessions[sessionID] = session
	am.mu.Unlock()

	return session
}

// GetSession retrieves a session by ID
func (am *AccountManager) GetSession(sessionID string) (*Session, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	session, ok := am.sessions[sessionID]
	if !ok {
		return nil, errors.New("session not found")
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, errors.New("session expired")
	}

	return session, nil
}

// InvalidateSession removes a session
func (am *AccountManager) InvalidateSession(sessionID string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.sessions, sessionID)
	log.Printf("Invalidated session: %s", sessionID)
}

// AccountExists checks if an account exists for a MAC address
func (am *AccountManager) AccountExists(macAddress string) (bool, error) {
	var exists int
	err := am.db.QueryRow(
		"SELECT COUNT(*) FROM accounts WHERE mac_address = ?",
		macAddress,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// AddUserBlocklist associates a blocklist with a user
func (am *AccountManager) AddUserBlocklist(macAddress, listName string) error {
	_, err := am.db.Exec(
		"INSERT OR IGNORE INTO user_blocklists (mac_address, list_name) VALUES (?, ?)",
		macAddress, listName,
	)
	if err != nil {
		return fmt.Errorf("failed to add user blocklist: %w", err)
	}
	log.Printf("Added blocklist %s for user %s", listName, macAddress)
	return nil
}

// RemoveUserBlocklist removes a blocklist association from a user
func (am *AccountManager) RemoveUserBlocklist(macAddress, listName string) error {
	_, err := am.db.Exec(
		"DELETE FROM user_blocklists WHERE mac_address = ? AND list_name = ?",
		macAddress, listName,
	)
	if err != nil {
		return fmt.Errorf("failed to remove user blocklist: %w", err)
	}
	log.Printf("Removed blocklist %s for user %s", listName, macAddress)
	return nil
}

// GetUserBlocklists returns all blocklists for a user
func (am *AccountManager) GetUserBlocklists(macAddress string) ([]string, error) {
	rows, err := am.db.Query(
		"SELECT list_name FROM user_blocklists WHERE mac_address = ? ORDER BY list_name",
		macAddress,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get user blocklists: %w", err)
	}
	defer rows.Close()

	var lists []string
	for rows.Next() {
		var listName string
		if err := rows.Scan(&listName); err != nil {
			return nil, err
		}
		lists = append(lists, listName)
	}

	return lists, rows.Err()
}

// cleanupSessions periodically removes expired sessions
func (am *AccountManager) cleanupSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		am.mu.Lock()
		now := time.Now()
		for id, session := range am.sessions {
			if now.After(session.ExpiresAt) {
				delete(am.sessions, id)
				log.Printf("Cleaned up expired session: %s", id)
			}
		}
		am.mu.Unlock()
	}
}

// generateSessionID creates a random session ID
func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// If random generation fails, use a combination of timestamp and fallback random
		log.Printf("Warning: crypto/rand failed, using fallback: %v", err)
		timestamp := time.Now().UnixNano()
		return fmt.Sprintf("%d%d", timestamp, timestamp%1000000)
	}
	return hex.EncodeToString(b)
}

// ChangePasscode allows a user to change their passcode
func (am *AccountManager) ChangePasscode(macAddress, oldPasscode, newPasscode string) error {
	if macAddress == "" || oldPasscode == "" || newPasscode == "" {
		return errors.New("all fields are required")
	}

	// Verify old passcode first
	var currentHash string
	err := am.db.QueryRow(
		"SELECT passcode_hash FROM accounts WHERE mac_address = ?",
		macAddress,
	).Scan(&currentHash)

	if err == sql.ErrNoRows {
		return errors.New("account not found")
	}
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(oldPasscode)); err != nil {
		return errors.New("invalid current passcode")
	}

	// Hash new passcode
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPasscode), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new passcode: %w", err)
	}

	// Update database
	_, err = am.db.Exec(
		"UPDATE accounts SET passcode_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE mac_address = ?",
		string(newHash), macAddress,
	)
	if err != nil {
		return fmt.Errorf("failed to update passcode: %w", err)
	}

	log.Printf("Changed passcode for MAC: %s", macAddress)
	return nil
}

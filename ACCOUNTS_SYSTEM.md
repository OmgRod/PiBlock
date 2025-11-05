# Account System Implementation

This document describes the MAC address-based account system implemented for PiBlock.

## Overview

The account system allows multiple users to have personalized blocklists and settings on the same PiBlock instance. Users are identified by their device MAC address rather than usernames, and each user can set a passcode to protect their settings.

## Key Features

### 1. MAC Address-Based Authentication
- Users are identified by their device's MAC address
- No usernames required - the MAC address serves as the unique identifier
- Each device gets its own account with personalized settings

### 2. Passcode Protection
- Users set a passcode (minimum 4 characters) when creating an account
- Passcodes are hashed using bcrypt for security
- Users can change their passcode from the Settings page

### 3. Guest Mode
- Users can choose to continue as guests without creating an account
- Guests have read-only access to:
  - View blocklists
  - View analytics
  - View logs
- Guests cannot:
  - Create or modify blocklists
  - Change settings
  - Delete lists or domains

### 4. Per-User Blocklists
- Each user's blocklists are stored separately with their MAC address prefix
- Format: `{MAC_ADDRESS}_{LIST_NAME}.txt`
- Only domains from a user's enabled blocklists are blocked for that user's device
- Different users can have completely different blocking policies

### 5. Session Management
- Sessions expire after 24 hours
- Session tokens are stored in localStorage on the client
- Sessions are cleaned up automatically on the server

### 6. DNS Filtering Per User
- When a user accesses the web interface, their IP address is mapped to their MAC address
- DNS queries from that IP are then filtered using only that user's blocklists
- This ensures each device only blocks the domains its owner configured

## Architecture

### Backend Components

#### 1. `accounts.go` - Account Manager
- Manages SQLite database with user accounts
- Handles account creation, authentication, and session management
- Stores MAC addresses and bcrypt-hashed passcodes
- Tracks user-to-blocklist associations

#### 2. `authapi.go` - Authentication API
- Provides endpoints for account operations:
  - `/auth/check` - Check if account exists for MAC address
  - `/auth/create` - Create new account with passcode
  - `/auth/login` - Login with passcode
  - `/auth/guest` - Create guest session
  - `/auth/logout` - Invalidate session
  - `/auth/verify` - Verify session validity
  - `/auth/change-passcode` - Change account passcode
- Caches IP-to-MAC mappings for DNS filtering

#### 3. `apihandlers.go` - API Request Handlers
- Handles blocklist operations with per-user filtering
- Implements authentication middleware
- Enforces guest read-only restrictions
- Includes security validations (path injection, SSRF protection)

#### 4. `macdetect.go` - MAC Detection
- Attempts to detect client MAC address from HTTP headers
- Falls back to IP address if MAC cannot be determined
- Normalizes MAC addresses to lowercase with colons

#### 5. `userfilter.go` - User-Specific DNS Filtering
- Caches IP-to-MAC mappings
- Checks if domains are blocked for specific users
- Used by DNS server for per-user query filtering

#### 6. `dnsserver.go` - DNS Server (Modified)
- Looks up MAC address from client IP
- Applies only that user's blocklists
- Records analytics per user

### Frontend Components

#### 1. `AuthGuard.jsx` - Authentication Flow
- Detects device MAC address (using browser fingerprint as fallback)
- Shows setup screen for new users
- Shows login screen for returning users
- Provides guest access option
- Injects session tokens into all API requests

#### 2. `App.jsx` - Settings Panel (Modified)
- Added logout button
- Added passcode change functionality
- Displays account management options

### Database Schema

```sql
-- User accounts
CREATE TABLE accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mac_address TEXT UNIQUE NOT NULL,
    passcode_hash TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- User blocklist associations
CREATE TABLE user_blocklists (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mac_address TEXT NOT NULL,
    list_name TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(mac_address, list_name),
    FOREIGN KEY (mac_address) REFERENCES accounts(mac_address) ON DELETE CASCADE
);
```

## User Flow

### First-Time User
1. User accesses PiBlock web interface
2. System detects device (MAC address or fingerprint)
3. User sees setup screen with two options:
   - Create account with passcode
   - Continue as guest
4. If creating account:
   - User enters and confirms passcode
   - Account is created and user is logged in
5. If choosing guest:
   - Guest session is created
   - User has read-only access

### Returning User
1. User accesses PiBlock web interface
2. System detects device MAC address
3. User sees login screen
4. User enters passcode or chooses guest mode
5. Upon successful login, user's session is restored

### Using the System
1. **Creating Blocklists**:
   - User creates list with name and URL or manual entries
   - List is stored as `{MAC}_{NAME}.txt`
   - List is automatically associated with user's account

2. **DNS Blocking**:
   - User's device makes DNS query
   - System looks up MAC address from IP
   - Only that user's blocklists are checked
   - Domain is blocked or allowed accordingly

3. **Analytics**:
   - All users can view analytics
   - Analytics show all queries (global view)
   - Future enhancement: Per-user analytics

## Security Considerations

### Implemented Protections
1. **Passcode Hashing**: bcrypt with default cost
2. **Session Tokens**: Cryptographically secure random tokens
3. **Path Injection Protection**: Input sanitization for file operations
4. **SSRF Protection**: URL validation, scheme checking, private IP blocking
5. **Guest Restrictions**: Read-only enforcement at API level

### Known Limitations

1. **MAC Detection**:
   - Browser-based detection uses fingerprinting (not true MAC)
   - Can be spoofed or cleared
   - Production should use server-side ARP detection

2. **NAT/IP Fallback**:
   - Devices behind NAT may share IP-based identifiers
   - Could allow account sharing within same network
   - Production should require proper MAC detection

3. **Session Storage**:
   - Sessions stored in memory (lost on restart)
   - Production should use persistent session store
   - Consider Redis or database-backed sessions

4. **No Account Recovery**:
   - Lost passcodes cannot be recovered
   - No email/phone verification
   - Production should add recovery mechanism

## Configuration

### Database Location
- Stored in `./data/accounts.db`
- Created automatically on first run
- Ensure `./data` directory is writable

### API Endpoints
- Auth API: `localhost:8082`
- Internal API: `localhost:8081`
- Frontend proxies both to web server on port 3000

### Session Duration
- Default: 24 hours
- Configurable in `accounts.go` (Session expiry)

## Future Enhancements

1. **Enhanced MAC Detection**:
   - Integration with DHCP server
   - ARP table monitoring
   - Network device integration

2. **Per-User Analytics**:
   - Track queries per user
   - Show user-specific statistics
   - Block history per device

3. **Account Recovery**:
   - Email-based recovery
   - Security questions
   - Admin override capability

4. **Multi-Device Accounts**:
   - Associate multiple MACs with one account
   - Family/group accounts
   - Device management interface

5. **Advanced Permissions**:
   - Admin accounts
   - Limited guest capabilities
   - Time-based restrictions

## Testing

### Manual Testing Steps

1. **Setup Flow**:
   - Clear browser storage
   - Access http://localhost:3000
   - Verify setup screen appears
   - Create account with passcode
   - Verify login successful

2. **Guest Mode**:
   - Access in incognito/private window
   - Choose guest mode
   - Verify read-only banner appears
   - Try to create list (should fail)
   - Verify analytics are visible

3. **List Management**:
   - Login as user
   - Create blocklist
   - Add domains
   - Verify list appears in dashboard
   - Login from another device
   - Verify list is not visible (per-user)

4. **Settings**:
   - Access settings page
   - Change passcode
   - Logout
   - Login with new passcode
   - Verify successful

## Troubleshooting

### Common Issues

1. **Cannot Login**:
   - Check that `data/accounts.db` exists and is writable
   - Verify auth API is running on port 8082
   - Check browser console for errors

2. **DNS Not Filtering**:
   - Ensure user accessed web interface first (to cache IP-to-MAC)
   - Check DNS server logs for user's IP
   - Verify blocklists are associated with user account

3. **Guest Banner Not Showing**:
   - Check session is properly created
   - Verify frontend is detecting guest status
   - Check browser console for session errors

4. **Database Errors**:
   - Ensure `./data` directory exists
   - Check file permissions
   - Verify SQLite is properly installed

## Maintenance

### Backup
- Backup `data/accounts.db` regularly
- Consider automated daily backups
- Store blocklist directory (`./blocklist/`) separately

### Cleanup
- Sessions auto-cleanup every hour
- Consider periodic database VACUUM
- Monitor database size growth

### Monitoring
- Check logs for authentication failures
- Monitor session creation/cleanup
- Track database query performance

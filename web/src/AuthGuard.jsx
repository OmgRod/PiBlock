import React, { useState, useEffect } from 'react'

// Utility to get the MAC address from the browser (simplified approach)
// NOTE: This is a demonstration implementation with known limitations:
// 1. Browser fingerprinting is not a reliable MAC address substitute
// 2. The generated identifier can be spoofed or cleared
// 3. In production, consider:
//    - Server-side MAC detection via ARP (requires local network)
//    - DHCP server integration
//    - Manual MAC entry with validation
//    - Client-side tools/extensions for MAC detection
async function getDeviceMAC() {
  // In a real implementation, this would need to be detected server-side
  // or provided by the user. For web apps, we can't directly access MAC.
  // We'll use a combination of fingerprinting for demo purposes
  
  // Try to get from localStorage (if user previously entered it)
  let mac = localStorage.getItem('device_mac')
  if (!mac) {
    // Generate a pseudo-MAC based on browser fingerprint
    // In production, this should be detected server-side via ARP
    const nav = navigator
    const screen = window.screen
    const fingerprint = [
      nav.userAgent,
      nav.language,
      screen.colorDepth,
      screen.width,
      screen.height,
      new Date().getTimezoneOffset()
    ].join('|')
    
    // Simple hash to create a MAC-like identifier
    let hash = 0
    for (let i = 0; i < fingerprint.length; i++) {
      hash = ((hash << 5) - hash) + fingerprint.charCodeAt(i)
      hash = hash & hash
    }
    
    // Convert to MAC format
    const hashStr = Math.abs(hash).toString(16).padStart(12, '0')
    mac = hashStr.match(/.{2}/g).join(':')
    localStorage.setItem('device_mac', mac)
  }
  
  return mac
}

export default function AuthGuard({ children }) {
  const [loading, setLoading] = useState(true)
  const [authenticated, setAuthenticated] = useState(false)
  const [isGuest, setIsGuest] = useState(false)
  const [showSetup, setShowSetup] = useState(false)
  const [showLogin, setShowLogin] = useState(false)
  const [passcode, setPasscode] = useState('')
  const [confirmPasscode, setConfirmPasscode] = useState('')
  const [error, setError] = useState('')
  const [macAddress, setMacAddress] = useState('')

  useEffect(() => {
    checkAuth()
  }, [])

  async function checkAuth() {
    setLoading(true)
    setError('')
    
    try {
      // Get device MAC
      const mac = await getDeviceMAC()
      setMacAddress(mac)
      
      // Check if session exists
      const sessionId = localStorage.getItem('session_id')
      if (sessionId) {
        const resp = await fetch('/auth/verify', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ session_id: sessionId })
        })
        
        if (resp.ok) {
          const data = await resp.json()
          setAuthenticated(true)
          setIsGuest(data.is_guest)
          setLoading(false)
          return
        } else {
          // Invalid session, clear it
          localStorage.removeItem('session_id')
        }
      }
      
      // Check if account exists
      const checkResp = await fetch('/auth/check', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          'X-Client-MAC': mac
        },
        body: JSON.stringify({ mac_address: mac })
      })
      
      if (checkResp.ok) {
        const data = await checkResp.json()
        if (data.exists) {
          setShowLogin(true)
        } else {
          setShowSetup(true)
        }
      } else {
        setError('Failed to check account status')
      }
    } catch (err) {
      setError('Connection error: ' + err.message)
    }
    
    setLoading(false)
  }

  async function handleSetup() {
    setError('')
    
    if (!passcode || passcode.length < 4) {
      setError('Passcode must be at least 4 characters')
      return
    }
    
    if (passcode !== confirmPasscode) {
      setError('Passcodes do not match')
      return
    }
    
    try {
      const resp = await fetch('/auth/create', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          'X-Client-MAC': macAddress
        },
        body: JSON.stringify({
          mac_address: macAddress,
          passcode: passcode
        })
      })
      
      if (resp.ok) {
        const data = await resp.json()
        localStorage.setItem('session_id', data.session_id)
        setAuthenticated(true)
        setIsGuest(false)
        setShowSetup(false)
      } else {
        const text = await resp.text()
        setError('Failed to create account: ' + text)
      }
    } catch (err) {
      setError('Connection error: ' + err.message)
    }
  }

  async function handleLogin() {
    setError('')
    
    if (!passcode) {
      setError('Please enter your passcode')
      return
    }
    
    try {
      const resp = await fetch('/auth/login', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          'X-Client-MAC': macAddress
        },
        body: JSON.stringify({
          mac_address: macAddress,
          passcode: passcode
        })
      })
      
      if (resp.ok) {
        const data = await resp.json()
        localStorage.setItem('session_id', data.session_id)
        setAuthenticated(true)
        setIsGuest(data.is_guest)
        setShowLogin(false)
      } else {
        setError('Invalid passcode')
      }
    } catch (err) {
      setError('Connection error: ' + err.message)
    }
  }

  async function handleGuest() {
    setError('')
    
    try {
      const resp = await fetch('/auth/guest', {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json',
          'X-Client-MAC': macAddress
        },
        body: JSON.stringify({
          mac_address: macAddress
        })
      })
      
      if (resp.ok) {
        const data = await resp.json()
        localStorage.setItem('session_id', data.session_id)
        setAuthenticated(true)
        setIsGuest(true)
        setShowLogin(false)
        setShowSetup(false)
      } else {
        setError('Failed to create guest session')
      }
    } catch (err) {
      setError('Connection error: ' + err.message)
    }
  }

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
        <div className="panel" style={{ maxWidth: 400, padding: 32 }}>
          <h2>Loading...</h2>
        </div>
      </div>
    )
  }

  if (!authenticated) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', padding: 16 }}>
        <div className="panel" style={{ maxWidth: 480, width: '100%', padding: 32 }}>
          <h1 style={{ marginBottom: 24 }}>PiBlock</h1>
          
          {showSetup && (
            <>
              <h2>Welcome! Set up your account</h2>
              <p className="muted" style={{ marginBottom: 16 }}>
                This device hasn't been set up yet. Create a passcode to secure your settings.
              </p>
              <p className="muted small" style={{ marginBottom: 16 }}>
                Device ID: {macAddress}
              </p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <input
                  type="password"
                  placeholder="Enter passcode (min 4 characters)"
                  value={passcode}
                  onChange={e => setPasscode(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && handleSetup()}
                  autoFocus
                />
                <input
                  type="password"
                  placeholder="Confirm passcode"
                  value={confirmPasscode}
                  onChange={e => setConfirmPasscode(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && handleSetup()}
                />
                {error && <div style={{ color: '#ff6b6b' }}>{error}</div>}
                <button className="btn" onClick={handleSetup}>Create Account</button>
                <button className="btn small" onClick={handleGuest}>Continue as Guest</button>
                <p className="muted small">As a guest, you can view blocklists and analytics but cannot make changes.</p>
              </div>
            </>
          )}
          
          {showLogin && (
            <>
              <h2>Welcome back!</h2>
              <p className="muted" style={{ marginBottom: 16 }}>
                Enter your passcode to continue.
              </p>
              <p className="muted small" style={{ marginBottom: 16 }}>
                Device ID: {macAddress}
              </p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <input
                  type="password"
                  placeholder="Enter passcode"
                  value={passcode}
                  onChange={e => setPasscode(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && handleLogin()}
                  autoFocus
                />
                {error && <div style={{ color: '#ff6b6b' }}>{error}</div>}
                <button className="btn" onClick={handleLogin}>Login</button>
                <button className="btn small" onClick={handleGuest}>Continue as Guest</button>
                <p className="muted small">As a guest, you can view blocklists and analytics but cannot make changes.</p>
              </div>
            </>
          )}
        </div>
      </div>
    )
  }

  // Inject session ID into all fetch requests
  // NOTE: This is a simple approach for demonstration. In production, consider:
  // - Using axios with interceptors
  // - React Context API for authentication state
  // - A dedicated HTTP client library
  const originalFetch = window.fetch
  window.fetch = function(...args) {
    const [url, options = {}] = args
    const sessionId = localStorage.getItem('session_id')
    
    if (sessionId && !options.headers) {
      options.headers = {}
    }
    if (sessionId) {
      if (options.headers instanceof Headers) {
        options.headers.set('X-Session-ID', sessionId)
      } else {
        options.headers['X-Session-ID'] = sessionId
      }
    }
    
    return originalFetch.call(this, url, options)
  }

  return (
    <>
      {isGuest && (
        <div style={{ 
          background: '#ff6b6b', 
          color: 'white', 
          padding: '8px 16px', 
          textAlign: 'center',
          fontSize: 14
        }}>
          You are viewing as a guest. Create an account to make changes.
        </div>
      )}
      {children}
    </>
  )
}

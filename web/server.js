const express = require('express')
const path = require('path')
const fetch = global.fetch || require('cross-fetch')

const app = express()
const PORT = process.env.PORT || 3000
// where Go internal API is expected to run
const GO_API = process.env.GO_API || 'http://127.0.0.1:8081'

app.use(express.json())

// Prevent caching in browsers: set strict no-cache headers on all responses.
app.use((req, res, next) => {
  res.setHeader('Cache-Control', 'no-store, no-cache, must-revalidate, proxy-revalidate')
  res.setHeader('Pragma', 'no-cache')
  res.setHeader('Expires', '0')
  next()
})

// proxy POST/GET/DELETE requests under /api/* to the Go internal API
// Proxy handler used for the public API routes the frontend currently calls
async function proxyHandler(req, res) {
  try {
    // forward the original path+query directly to the Go internal API
    const targetPath = req.originalUrl
    const url = GO_API + targetPath
    const opts = { method: req.method, headers: {} }
    // forward most headers from the incoming request, except host
    opts.headers = Object.assign({}, req.headers)
    delete opts.headers.host
    // If body parser produced a body, forward it. For non-JSON content types this may need extension.
    if (req.body && Object.keys(req.body).length) {
      try {
        opts.body = JSON.stringify(req.body)
        if (!opts.headers['content-type']) opts.headers['content-type'] = 'application/json'
      } catch (e) {
        // fallback: ignore body
      }
    }

    const response = await fetch(url, opts)
    // copy response headers to the client, but ensure no-cache remains
    response.headers.forEach((v, k) => {
      try { res.setHeader(k, v) } catch (e) { /* ignore */ }
    })
    // override caching headers to disable browser caching
    res.setHeader('Cache-Control', 'no-store, no-cache, must-revalidate, proxy-revalidate')
    res.setHeader('Pragma', 'no-cache')
    res.setHeader('Expires', '0')

    const buf = await response.buffer()
    res.status(response.status).send(buf)
  } catch (err) {
    res.status(500).send(err.message)
  }
}

// Proxy the routes used by the frontend directly so existing fetch calls
// (e.g. fetch('/lists')) work without changing the frontend.
const apiRoutes = ['/lists', '/lists/*', '/analytics', '/validate', '/reload', '/check', '/logs']
apiRoutes.forEach(p => app.use(p, proxyHandler))

// keep legacy /api prefix support
app.use('/api', proxyHandler)

// serve built static files if available
const distDir = path.join(__dirname, 'dist')
app.use(express.static(distDir, {
  setHeaders: (res, filePath) => {
    // ensure static assets are not cached by the browser during development
    res.setHeader('Cache-Control', 'no-store, no-cache, must-revalidate, proxy-revalidate')
    res.setHeader('Pragma', 'no-cache')
    res.setHeader('Expires', '0')
  }
}))

// fallback to index.html
// fallback for SPA; use '/*' to avoid path-to-regexp issues with a bare '*'
app.get('/*', (req, res) => {
  // Serve index.html for SPA routing, ensure no-cache headers are set
  res.setHeader('Cache-Control', 'no-store, no-cache, must-revalidate, proxy-revalidate')
  res.setHeader('Pragma', 'no-cache')
  res.setHeader('Expires', '0')
  res.sendFile(path.join(distDir, 'index.html'))
})

app.listen(PORT, () => console.log(`Frontend server listening on http://localhost:${PORT}, proxying to ${GO_API}`))

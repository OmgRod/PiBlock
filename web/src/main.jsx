import React from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import AuthGuard from './AuthGuard'
import './styles.scss'

createRoot(document.getElementById('root')).render(
  <AuthGuard>
    <App />
  </AuthGuard>
)

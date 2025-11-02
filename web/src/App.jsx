import React, { useEffect, useState } from 'react'
import './styles.css'
import { Container, Navbar, Nav, Button, Form, Row, Col, Card } from 'react-bootstrap'
import { BsPlus, BsTrash, BsDownload, BsGear } from 'react-icons/bs'

function ListsPanel({ onNavigate }) {
  const [lists, setLists] = useState({})
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [itemsText, setItemsText] = useState('')
  useEffect(() => { refresh() }, [])
  const refresh = () => { fetch('/lists').then(r => r.json()).then(setLists).catch(()=>setLists({})) }
  const createFromUrl = async () => {
    if (!name || !url) { window.alert('Please provide name and url'); return }
    try{
      const resp = await fetch('/lists/create', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ name, url }) })
      if (!resp.ok) { const t = await resp.text(); throw new Error(t) }
      const txt = await resp.text()
      window.alert(txt)
      setName(''); setUrl(''); refresh()
    }catch(e){ window.alert('Error: '+e) }
  }
  const createFromItems = async () => {
    if (!name || !itemsText) { window.alert('Please provide name and items'); return }
    try{
      // send items as a single string; server will split on commas/spaces/newlines
      const resp = await fetch('/lists/create', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ name, items: itemsText }) })
      if (!resp.ok) { const t = await resp.text(); throw new Error(t) }
      const txt = await resp.text()
      window.alert(txt)
      setName(''); setItemsText(''); refresh()
    }catch(e){ window.alert('Error: '+e) }
  }
  return (
    <div className="panel">
      <div className="panel-header">
        <h2>Blocklists</h2>
        <button onClick={refresh} className="btn small">Refresh</button>
      </div>
      <div style={{marginBottom:10}}>
        <div style={{display:'flex',gap:8,flexWrap:'wrap',alignItems:'center'}}>
          <input placeholder="list name" value={name} onChange={e=>setName(e.target.value)} />
          <input placeholder="url (optional)" style={{flex:1}} value={url} onChange={e=>setUrl(e.target.value)} />
          <button className="btn small" onClick={createFromUrl}>Create from URL</button>
        </div>
        <div style={{display:'flex',gap:8,marginTop:8}}>
          <textarea placeholder="paste domains (comma, space or newline separated)" value={itemsText} onChange={e=>setItemsText(e.target.value)} style={{flex:1,minHeight:60}} />
          <div style={{display:'flex',flexDirection:'column',gap:8}}>
            <button className="btn small" onClick={createFromItems}>Create from items</button>
          </div>
        </div>
      </div>
      <div className="lists-grid">
        {Object.keys(lists).length === 0 && <div className="muted">No lists found</div>}
        {Object.entries(lists).map(([name, count]) => (
          <div key={name} className="list-card" onClick={() => onNavigate(name)}>
            <div className="list-name">{name}</div>
            <div className="list-count">{count} items</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function ListDetail({ name, onClose, onRemoved }) {
  const [items, setItems] = useState([])
  const [offset, setOffset] = useState(0)
  const [limit, setLimit] = useState(100)
  const [total, setTotal] = useState(0)
  const [q, setQ] = useState('')

  const fetchPage = (search) => {
    const qs = new URLSearchParams({ offset: String(offset), limit: String(limit), q: search || q })
    fetch(`/lists/items/${encodeURIComponent(name)}?${qs.toString()}`)
      .then(r => r.json())
      .then(j => { setItems(j.items || []); setTotal(j.total || 0) })
      .catch(() => { setItems([]); setTotal(0) })
  }

  useEffect(() => { if (name) fetchPage() }, [name, offset, limit])

  const doSearch = () => { setOffset(0); fetchPage(q) }
  const prev = () => { setOffset(Math.max(0, offset - limit)) }
  const next = () => { if (offset+limit < total) setOffset(offset+limit) }
  const remove = (domain) => {
    if (!window.confirm(`Remove ${domain} from ${name}?`)) return
    fetch(`/lists/items/${encodeURIComponent(name)}`, {
      method: 'DELETE', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ domain })
    }).then(r => {
      if (r.ok) { onRemoved && onRemoved(); fetchPage(); }
      else r.text().then(t => window.alert('Error: '+t))
    }).catch(e => window.alert('Error: '+e))
  }
  const showingFrom = Math.min(total, offset+1)
  const showingTo = Math.min(total, offset+items.length)

  return (
    <div className="panel detail">
      <div className="panel-header">
        <h2>{name}</h2>
        <div>
          <button className="btn small" onClick={onClose}>Close</button>
        </div>
      </div>
      <div className="list-controls">
        <div className="muted">Showing {showingFrom} to {showingTo} of {total} websites blocked</div>
        <div className="search-row">
          <input placeholder="search domain" value={q} onChange={e=>setQ(e.target.value)} />
          <button className="btn" onClick={doSearch}>Search</button>
        </div>
      </div>
      <div style={{marginTop:8, marginBottom:8}}>
        <div className="muted">Append domains to this list (comma/space/newline separated)</div>
        <div style={{display:'flex',gap:8,marginTop:6}}>
          <input id={`append-${name}`} placeholder="domains to append" style={{flex:1}} />
          <button className="btn small" onClick={()=>{
            const v = document.getElementById(`append-${name}`).value
            if (!v) { window.alert('Enter domains to append'); return }
            fetch(`/lists/${encodeURIComponent(name)}/append`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ items: v }) })
              .then(r=>{ if (r.ok) { window.alert('Appended'); fetchPage(); document.getElementById(`append-${name}`).value=''} else r.text().then(t=>window.alert('Error: '+t)) })
              .catch(e=>window.alert('Error: '+e))
          }}>Append</button>
        </div>
      </div>
      <div className="list-items">
        {items.map((it, idx) => (
          <div key={idx} className="list-item">
            <div className="domain">{it}</div>
            <div><button className="btn tiny danger" onClick={()=>remove(it)}>Remove</button></div>
          </div>
        ))}
      </div>
      <div className="pager">
        <button className="btn" onClick={prev} disabled={offset===0}>Prev</button>
        <button className="btn" onClick={next} disabled={offset+limit>=total}>Next</button>
      </div>
    </div>
  )
}

function AnalyticsPanel() {
  const [analytics, setAnalytics] = useState(null)
  const [error, setError] = useState(null)
  const loadAnalytics = ()=>{
    setError(null)
    fetch('/analytics').then(r=>{
      if (!r.ok) throw new Error('status '+r.status)
      return r.json()
    }).then(j=>{ setAnalytics(j); setError(null) }).catch(e=>{ setAnalytics(null); setError(e.message || String(e)) })
  }
  useEffect(()=>{ loadAnalytics() }, [])
  if (!analytics && !error) return <div className="panel"><h2>Analytics</h2><div>Loading...</div></div>
  if (error) return <div className="panel"><h2>Analytics</h2><div className="muted">Failed to load analytics: {error}</div><div style={{marginTop:8}}><Button onClick={loadAnalytics}>Retry</Button></div></div>

  // utility: simple pie chart (two slices) using inline SVG
  const PieChart = ({values, labels, size=120, colors=['#38bdf8','#ff6b6b']}) => {
    const total = values.reduce((s,v)=>s+v,0) || 1
    let angle = 0
    const center = size/2
    const radius = center - 2
    const paths = values.map((v,i)=>{
      const slice = v/total
      const a = slice * Math.PI*2
      const x = center + radius * Math.cos(angle + a)
      const y = center + radius * Math.sin(angle + a)
      const large = a > Math.PI ? 1 : 0
      const d = `M ${center} ${center} L ${center + radius*Math.cos(angle)} ${center + radius*Math.sin(angle)} A ${radius} ${radius} 0 ${large} 1 ${x} ${y} Z`
      angle += a
      return <path key={i} d={d} fill={colors[i%colors.length]} />
    })
    return <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="pie">{paths}</svg>
  }

  const BarChart = ({items, width=400, height=120}) => {
    const max = Math.max(...items.map(i=>i[1]), 1)
    const barH = Math.floor(height / items.length)
    return (
      <svg width={width} height={height} className="barchart">
        {items.map((it, idx) => {
          const w = Math.round((it[1]/max) * (width-120))
          const y = idx * barH
          return (
            <g key={idx} transform={`translate(0, ${y})`}>
              <text x={0} y={barH/2+4} fontSize={12} fill="#cbd5e1">{it[0]}</text>
              <rect x={120} y={2} width={w} height={barH-6} fill="#38bdf8" />
              <text x={120+w+6} y={barH/2+4} fontSize={12} fill="#cbd5e1">{it[1]}</text>
            </g>
          )
        })}
      </svg>
    )
  }

  const total = analytics.queries || 0
  const blocked = analytics.blocked || 0
  const allowed = Math.max(0, total - blocked)
  const domainHits = analytics.domain_hits || {}
  const top = Object.entries(domainHits).sort((a,b)=>b[1]-a[1]).slice(0,10)
  const clientHitsEntries = Object.entries(analytics.client_hits || {}).sort((a,b)=>b[1]-a[1]).slice(0,10)

  return (
    <div className="panel">
      <h2>Analytics</h2>
      <div style={{display:'flex',gap:16,flexWrap:'wrap'}}>
        <div style={{minWidth:220}}>
          <h4>Traffic</h4>
          <div style={{display:'flex',alignItems:'center',gap:12}}>
            <PieChart values={[blocked, allowed]} labels={["Blocked","Allowed"]} />
            <div>
              <div>Total queries: <strong>{total}</strong></div>
              <div>Blocked: <strong>{blocked}</strong></div>
              <div>Allowed: <strong>{allowed}</strong></div>
            </div>
          </div>
        </div>

        <div style={{flex:1,minWidth:320}}>
          <h4>Top blocked domains</h4>
          {top.length === 0 && <div className="muted">No blocked domains yet</div>}
          {top.length > 0 && <BarChart items={top} width={520} height={top.length*28} />}
        </div>
      </div>

      <div style={{marginTop:16}}>
        <h4>Top clients</h4>
        {clientHitsEntries.length === 0 && <div className="muted">No client data yet</div>}
        {clientHitsEntries.length > 0 && (
          <ul>
            {clientHitsEntries.map(([c,n]) => <li key={c}>{c} — {n}</li>)}
          </ul>
        )}
      </div>
    </div>
  )
}

function LogsPanel(){
  const [logs, setLogs] = useState([])
  const [auto, setAuto] = useState(true)
  const [intervalSec, setIntervalSec] = useState(5)
  const [limit, setLimit] = useState(200)

  const refresh = () => fetch(`/logs?limit=${limit}`).then(r=>r.json()).then(setLogs).catch(()=>setLogs([]))

  useEffect(()=>{ refresh() }, [limit])

  useEffect(()=>{
    if (!auto) return
    const t = setInterval(refresh, Math.max(1000, intervalSec*1000))
    return ()=>clearInterval(t)
  }, [auto, intervalSec, limit])

  const clearLogs = ()=>{
    if (!window.confirm('Clear persistent logs and recent in-memory logs?')) return
    fetch('/logs', { method:'DELETE' }).then(r=>{
      if (r.ok) { refresh(); window.alert('Logs cleared') }
      else r.text().then(t=>window.alert('Failed: '+t))
    }).catch(e=>window.alert('Error: '+e))
  }

  const downloadLogs = async ()=>{
    try{
      const resp = await fetch(`/logs?limit=10000`)
      const json = await resp.json()
      const blob = new Blob([JSON.stringify(json, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'piblock-logs.json'
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    }catch(e){ window.alert('Failed to download logs: '+e) }
  }

  return (
    <div className="panel">
      <div className="panel-header">
        <h2>Recent Logs</h2>
        <div style={{display:'flex',gap:8}}>
          <button className="btn small" onClick={refresh}>Refresh</button>
          <button className="btn small" onClick={downloadLogs}>Download</button>
          <button className="btn small btn-danger" onClick={clearLogs}>Clear</button>
        </div>
      </div>
      <div className="list-controls" style={{marginBottom:8}}>
        <div style={{display:'flex',gap:8,alignItems:'center',flexWrap:'wrap'}}>
          <label className="muted small">Auto-refresh</label>
          <input type="checkbox" checked={auto} onChange={e=>setAuto(e.target.checked)} />
          <label className="muted small">Interval (s)</label>
          <input style={{width:80}} type="number" value={intervalSec} min={1} onChange={e=>setIntervalSec(Number(e.target.value)||1)} />
          <label className="muted small">Limit</label>
          <input style={{width:100}} type="number" value={limit} min={10} onChange={e=>setLimit(Number(e.target.value)||100)} />
        </div>
      </div>
      <div className="logs">
        {logs.length===0 && <div className="muted">No logs</div>}
        {logs.map((l, idx) => {
          const timeStr = l.time ? new Date(l.time).toLocaleString() : (l.Time ? new Date(l.Time).toLocaleString() : '—')
          const client = l.client || l.Client || '—'
          const domain = l.domain || l.Domain || '—'
          const blocked = (typeof l.blocked !== 'undefined') ? l.blocked : (typeof l.Blocked !== 'undefined' ? l.Blocked : false)
          return (
            <div key={idx} className="log-row small">{timeStr} — {client} — {domain} — {blocked ? 'BLOCKED' : 'OK'}</div>
          )
        })}
      </div>
    </div>
  )
}

export default function App(){
  const [view, setView] = useState('lists')
  const [selected, setSelected] = useState(null)
  const [refreshFlag, setRefreshFlag] = useState(0)
  return (
    <div className="app">
      <Navbar bg="dark" variant="dark" className="mb-2">
        <Container fluid>
          <Navbar.Brand>PiBlock</Navbar.Brand>
          <Nav>
            <Nav.Link active={view==='lists'} onClick={()=>setView('lists')}>Lists</Nav.Link>
            <Nav.Link active={view==='analytics'} onClick={()=>setView('analytics')}>Analytics</Nav.Link>
            <Nav.Link active={view==='logs'} onClick={()=>setView('logs')}>Logs</Nav.Link>
            <Nav.Link active={view==='settings'} onClick={()=>setView('settings')}><BsGear/></Nav.Link>
          </Nav>
        </Container>
      </Navbar>
      <main>
        {view === 'lists' && !selected && <ListsPanel onNavigate={(name)=>{ setSelected(name); setView('lists') }} />}
        {view === 'lists' && selected && <ListDetail name={selected} onClose={()=>{ setSelected(null); setRefreshFlag(f=>f+1) }} onRemoved={()=>setRefreshFlag(f=>f+1)} />}
        {view === 'analytics' && <AnalyticsPanel />}
        {view === 'logs' && <LogsPanel />}
        {view === 'settings' && <SettingsPanel />}
      </main>
    </div>
  )
}

function SettingsPanel(){
  const [apiBase, setApiBase] = useState('/')
  const [showAdvanced, setShowAdvanced] = useState(false)

  useEffect(()=>{
    // nothing heavy here yet; placeholder to later fetch config from API
  }, [])

  return (
    <div className="panel">
      <div className="panel-header"><h2>Settings</h2></div>
      <div className="list-controls">
        <div className="muted">Basic settings for PiBlock. More options (upstream DNS, whitelist, gravity) will be added.</div>
        <div style={{marginTop:8}}>
          <label className="small muted">API base</label>
          <div style={{display:'flex',gap:8,marginTop:6}}>
            <input value={apiBase} onChange={e=>setApiBase(e.target.value)} />
            <button className="btn small">Save</button>
          </div>
        </div>
        <div style={{marginTop:12}}>
          <label className="small muted">Advanced</label>
          <div style={{marginTop:6}}>
            <label><input type="checkbox" checked={showAdvanced} onChange={e=>setShowAdvanced(e.target.checked)} /> Show advanced options</label>
          </div>
        </div>
      </div>
    </div>
  )
}

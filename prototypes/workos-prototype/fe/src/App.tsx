import { BrowserRouter, Routes, Route, Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { getMe, logout, type Me } from './api'
import Home from './pages/Home'
import Public from './pages/Public'
import AdminOnly from './pages/AdminOnly'
import './App.css'

function Header({ me, onLogout }: { me: Me | null; onLogout: () => void }) {
  return (
    <header style={{ padding: 16, borderBottom: '1px solid #ddd', display: 'flex', gap: 16, alignItems: 'center' }}>
      <strong>WorkOS Prototype</strong>
      <nav style={{ display: 'flex', gap: 12 }}>
        <Link to="/">Home</Link>
        <Link to="/public">Public</Link>
        <Link to="/admin-only">Admin Only</Link>
      </nav>
      <span style={{ marginLeft: 'auto' }}>
        {me ? (
          <>
            <span style={{ marginRight: 12 }}>
              {me.email} {me.role && <em>({me.role})</em>}
            </span>
            <button onClick={onLogout}>Sign out</button>
          </>
        ) : (
          <a href="/login"><button>Sign in</button></a>
        )}
      </span>
    </header>
  )
}

export default function App() {
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)

  async function refresh() {
    try {
      const m = await getMe()
      setMe(m)
    } catch {
      setMe(null)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { refresh() }, [])

  async function handleLogout() {
    try { await logout() } catch { /* ignore */ }
    setMe(null)
    window.location.href = '/'
  }

  if (loading) return <p style={{ padding: 24 }}>Loading…</p>

  return (
    <BrowserRouter>
      <Header me={me} onLogout={handleLogout} />
      <main style={{ padding: 24 }}>
        <Routes>
          <Route path="/" element={<Home me={me} />} />
          <Route path="/public" element={<Public />} />
          <Route path="/admin-only" element={<AdminOnly />} />
        </Routes>
      </main>
    </BrowserRouter>
  )
}

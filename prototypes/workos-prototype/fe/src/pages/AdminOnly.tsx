import { useEffect, useState } from 'react'
import { getAdminOnly } from '../api'

export default function AdminOnly() {
  const [data, setData] = useState<{ message: string; viewer: string; role: string } | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getAdminOnly().then(setData).catch(e => setError(String(e)))
  }, [])

  if (error)
    return (
      <section>
        <h1>Admin Only</h1>
        <p>Request was rejected:</p>
        <pre style={{ color: 'crimson' }}>{error}</pre>
        <p><em>This is the expected behavior if your role is not <code>admin</code>.</em></p>
      </section>
    )
  if (!data) return <p>Loading…</p>

  return (
    <section>
      <h1>Admin Only</h1>
      <p>{data.message}</p>
      <p><strong>Viewer:</strong> {data.viewer} ({data.role})</p>
    </section>
  )
}

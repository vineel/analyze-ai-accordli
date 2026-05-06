import { useEffect, useState } from 'react'
import { getPublic } from '../api'

export default function Public() {
  const [data, setData] = useState<{ message: string; viewer: string; role: string } | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getPublic().then(setData).catch(e => setError(String(e)))
  }, [])

  if (error) return <pre style={{ color: 'crimson' }}>{error}</pre>
  if (!data) return <p>Loading…</p>

  return (
    <section>
      <h1>Public resource</h1>
      <p>{data.message}</p>
      <p><strong>Viewer:</strong> {data.viewer} ({data.role})</p>
    </section>
  )
}

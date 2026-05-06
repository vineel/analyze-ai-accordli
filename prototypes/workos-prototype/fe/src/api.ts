// Thin fetch wrappers. All requests use cookies (credentials: 'include')
// even though we're same-origin via Vite proxy — explicit for clarity.

export type Me = {
  user_id: string
  email: string
  first_name?: string
  last_name?: string
  organization_id?: string
  role?: string
  expires_at: string
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { credentials: 'include', ...init })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`${res.status} ${res.statusText}: ${body}`)
  }
  return res.json()
}

export const getMe = () => request<Me>('/api/me')

export const getPublic = () =>
  request<{ message: string; viewer: string; role: string }>('/api/public')

export const getAdminOnly = () =>
  request<{ message: string; viewer: string; role: string }>('/api/admin-only')

export const logout = () =>
  request<{ ok: boolean }>('/logout', { method: 'POST' })

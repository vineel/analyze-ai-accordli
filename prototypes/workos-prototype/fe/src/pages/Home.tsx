import type { Me } from '../api'

export default function Home({ me }: { me: Me | null }) {
  if (!me) {
    return (
      <section>
        <h1>Welcome</h1>
        <p>You're not signed in.</p>
        <a href="/login"><button>Sign in</button></a>
      </section>
    )
  }

  return (
    <section>
      <h1>Signed in</h1>
      <dl style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: 8 }}>
        <dt><strong>Email</strong></dt><dd>{me.email}</dd>
        <dt><strong>User ID</strong></dt><dd><code>{me.user_id}</code></dd>
        <dt><strong>Name</strong></dt><dd>{[me.first_name, me.last_name].filter(Boolean).join(' ') || <em>(none)</em>}</dd>
        <dt><strong>Organization ID</strong></dt><dd>{me.organization_id ? <code>{me.organization_id}</code> : <em>(none — sign in via an org-scoped flow)</em>}</dd>
        <dt><strong>Role</strong></dt><dd>{me.role || <em>(none)</em>}</dd>
        <dt><strong>Session expires</strong></dt><dd>{new Date(me.expires_at).toLocaleString()}</dd>
      </dl>
    </section>
  )
}

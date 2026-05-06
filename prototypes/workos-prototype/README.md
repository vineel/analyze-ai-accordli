# WorkOS Prototype

A minimal Go + Vite/React/TS prototype that exercises WorkOS AuthKit:

- Login (email+password and Google OAuth)
- Multi-tenant Organizations
- Roles + protected routes (`admin` vs `member`)

Stack mirrors the Accordli Contract Workbench so the learnings transfer.

---

## First-time setup (do once)

### 1. Create the `.env` file

```bash
cp .env.example .env
```

Fill in:

- `WORKOS_CLIENT_ID` and `WORKOS_API_KEY` — from WorkOS dashboard → Developer → API Keys.
- `WORKOS_COOKIE_PASSWORD` — generate a random 32+ char string:

  ```bash
  openssl rand -base64 32
  ```

  Paste the output as the value.

Leave the other values at their defaults.

### 2. Verify dashboard configuration matches

In your WorkOS dashboard:

- **Developer → Redirects** → Sign-in callback = `http://localhost:8080/auth/callback`
- **Developer → Redirects** → Sign-in endpoint = `http://localhost:8080/login`
- **Developer → Redirects** → Sign-out redirect = `http://localhost:5173/`
- **Products → Authentication** → Email+password and Google OAuth enabled
- **Organizations** → at least one organization exists with you as a member, role `admin`

---

## Run

Two terminals.

**Terminal 1 — backend:**

```bash
cd be
go run ./cmd/api
```

Listens on `http://localhost:8080`.

**Terminal 2 — frontend:**

```bash
cd fe
npm install   # first run only
npm run dev
```

Serves on `http://localhost:5173` and proxies `/api`, `/auth`, `/login`, `/logout` to the backend.

Open `http://localhost:5173` in a browser.

---

## Test sequence

1. **Sign in**: click the "Sign in" button. AuthKit's hosted login page appears at `upright-rule-77-staging.authkit.app`. Sign in with Google or email+password.
2. **Home page** displays your `email`, `user_id`, `organization_id`, and `role`.
3. **`/public`** — should always succeed for any signed-in user.
4. **`/admin-only`** — succeeds if your role is `admin`, returns 403 otherwise.
5. **Sign out** — clears the cookie. Refreshing the home page should now show the sign-in button again.

To exercise the multi-tenancy story, sign in as users assigned to different Organizations and confirm `organization_id` in the home page changes accordingly. To exercise the role check, swap a user's role in the WorkOS dashboard between `admin` and `member` and watch `/admin-only`'s 403 flip.

---

## File map

```
.
├── .env.example                    # template; copy to .env and fill in
├── README.md                       # you're reading it
├── be/                             # Go backend (gin)
│   ├── go.mod
│   ├── cmd/api/main.go             # entrypoint, route table, env loading
│   └── internal/
│       ├── auth/
│       │   ├── session.go          # AES-GCM seal/unseal + JWT claim parsing
│       │   └── middleware.go       # gin middleware + role guard
│       └── handlers/
│           ├── auth.go             # /login, /auth/callback, /logout
│           ├── me.go               # /api/me
│           ├── public.go           # /api/public — any signed-in user
│           └── admin.go            # /api/admin-only — admin role only
└── fe/                             # Vite + React + TS frontend
    ├── vite.config.ts              # /api,/auth,/login,/logout → :8080
    └── src/
        ├── App.tsx                 # router, header, sign-in/out
        ├── api.ts                  # fetch wrappers
        └── pages/
            ├── Home.tsx
            ├── Public.tsx
            └── AdminOnly.tsx
```

---

## Notes & caveats

- **Cookie cryptography**: AES-256-GCM, 24-hour expiry. Custom-rolled rather than using a WorkOS Go session helper because the v6 Go SDK exposes auth primitives but leaves session sealing to the application.
- **JWT signature verification**: skipped on access-token claim parsing. WorkOS issues these tokens; for a prototype we trust them. Production code should verify signatures against the WorkOS JWKS endpoint.
- **Multi-org users**: AuthKit's default behavior is to issue one access token per active organization. If a user belongs to multiple orgs, switching active org would require a re-authentication or a separate "select organization" flow — out of scope here.
- **Logout completeness**: `/logout` clears the local cookie but does not sign the user out of AuthKit / Google. Add a redirect to WorkOS's logout URL if you need a full sign-out.
- **HTTPS**: cookies are set with `Secure=false` because we're on `http://localhost`. Flip to true when deploying behind TLS.

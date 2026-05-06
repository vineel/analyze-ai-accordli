# Phase 0 — Kickoff

Companion to `solomocky-to-mocky-plan.md` §"Phase 0 — Local dev tooling". This file is the running checklist for executing it. Two tracks: **(A) human actions** that need account creation / DNS / installs on each developer's laptop, and **(B) code changes** in the repo. Both run in parallel.

## Deviation from the written plan

- **Postgres 18 local, not Postgres 17 in docker.** Vineel called this. Reasons: PG 18 is GA on Azure Flexible Server as of December 2025, brew installs work fine locally, and a one-fewer-moving-piece local Postgres beats docker-compose for this team. PG 18 is the version for everything from here through prod.

Everything else in Phase 0 stands.

---

## Track A — Human actions

These need real accounts on third-party services. **Bold = blocking** for the Phase 0 exit criterion ("receive a webhook from Stripe Dashboard or WorkOS test panel at the tunnel URL"). The rest can land during Phase 1/4/5 prep.

### A1. Tailscale tailnet + Funnel — **blocking**

We're using **Tailscale Funnel** to expose each developer's local API to the public internet at a stable hostname. Funnel is free on the Personal plan (up to 6 users, unlimited devices) and is the lightest path: no DNS zone admin, no separate cert config, no per-tunnel credentials file.

Tradeoff: hostnames are forced into `<machine>.<tailnet>.ts.net`. Custom strings (e.g. `accordli.ts.net`) require a paid plan + verified domain. On the free plan the tailnet name is either the auto-generated `tail<hex>` or a one-shot pick from Tailscale's preset "fun" word list (animals/colors). Our actual tailnet today is `tail9acde7`. We're sticking with it for Phase 0 — the URL only shows up in webhook configs and a couple of places in `.env`. If we ever want something readable, the rename is a one-click admin change but only works once.

Funnel only listens publicly on ports **443, 8443, or 10000**, but it forwards to any local port. `sudo tailscale funnel --bg 8080` puts our API on `https://<host>/`.

**One-time setup, owned by Vineel (the tailnet admin):**

1. **Create the tailnet.** Go to <https://login.tailscale.com> and sign up via SSO (Google / GitHub / Microsoft / Okta — whichever has the long-term Accordli identity). The tailnet is created on first sign-in.
2. **Tailnet DNS name.** Default is `tail<hex>` (currently `tail9acde7`). On the free plan you can optionally rename to a single preset "fun" name (admin console → Settings → General → "Tailnet name") — one rename is permitted. Custom strings like `accordli.ts.net` require a paid plan with a verified domain. Phase 0 stays on `tail9acde7`; revisit if we move to a paid Tailscale tier.
3. **Invite Tom** in Users → Invite users.
4. **Verify ACL.** Tailscale ships a default ACL that allows Funnel for `autogroup:member`. If the policy was edited, ensure the `funnel` node attribute remains granted. No change needed for greenfield tailnets.

**Per-developer setup (Vineel and Tom each do this on their laptop):**

1. **Install Tailscale.** Either Mac App Store (recommended for macOS — the app handles the daemon) or `brew install --cask tailscale`. Linux/Windows variants are documented at <https://tailscale.com/download> if anyone joins later.
2. **Sign in.** Launch the app (or `tailscale up`) and authenticate to the Accordli tailnet via the same SSO provider Vineel set up.
3. **Rename the machine** in the Tailscale admin console (Machines → click row → "Edit machine name") to something stable like `vineel-dev-ds9` or `tom-dev-<hostname>`. The OS hostname is the default and may not be what you want as a public URL component.
4. **Set `TUNNEL_HOSTNAME` in `.env`** to the resulting `https`-able host, e.g.:
   ```
   TUNNEL_HOSTNAME=vineel-dev-ds9.tail9acde7.ts.net
   ```
   This is the hostname `make dev` echoes; `make tunnel` doesn't read it (Tailscale figures the host out from the local node identity).
5. **Test:** `make tunnel` (added in Track B), then `curl https://$TUNNEL_HOSTNAME/api/health` → expect the API health JSON.

To turn the funnel off: `sudo tailscale funnel --bg off` (or `tailscale funnel reset`).

### A2. WorkOS staging environment — **blocking** (for the exit-criterion webhook test, if going via WorkOS rather than Stripe)

One shared environment, multiple webhook endpoints (one per developer's tunnel).

1. Vineel creates the WorkOS account at <https://workos.com> if not already done. Use a shared mailbox (`accounts@accordli.com` if it exists; otherwise Vineel's email is fine — switchable later).
2. In the dashboard, create a **Staging environment**. Production will be a separate environment in Phase 8.
3. Grab the **API key** (starts with `sk_test_…`) and the **client ID**. These are *shared* across developers — both go in `.env.example` as placeholders, real values in each dev's `.env`.
4. **Webhook endpoints**: Settings → Webhooks → "Add endpoint". One per developer:
   - `https://vineel-dev-ds9.tail9acde7.ts.net/webhooks/workos`
   - `https://tom-dev-<host>.tail9acde7.ts.net/webhooks/workos`
   - WorkOS fan-outs to all configured endpoints. Each dev gets the full event stream.
5. Capture the **webhook signing secret** (per-endpoint, `whsec_…`). Each dev's `.env` carries their own.
6. **AuthKit redirect URIs**: AuthKit → Configuration. Add per-developer:
   - `https://vineel-dev-ds9.tail9acde7.ts.net/auth/callback`
   - `https://tom-dev-<host>.tail9acde7.ts.net/auth/callback`
7. Add Tom as a team member on the WorkOS dashboard so he can self-serve.

### A3. Stripe staging account — **blocking** (the easier path for the exit-criterion webhook test)

1. Vineel creates the Stripe account at <https://stripe.com> if not already done. **Toggle to test mode** (top-left) and stay there for all of Phase 0–7 work.
2. **API keys** (Developers → API keys): publishable + secret (`sk_test_…`). Shared values; each dev pastes the same secret into their `.env`.
3. **Stripe CLI**: each developer runs `brew install stripe/stripe-cli/stripe` and then `stripe login` (browser auth links the CLI to the test account).
4. **Webhook signing secret for the CLI**: shown by `stripe listen --forward-to localhost:8080/webhooks/stripe` on first run. Distinct from a "real" webhook endpoint — the CLI mints an ephemeral one. Paste into `.env` as `STRIPE_WEBHOOK_SECRET_CLI`.
5. **Optional persistent webhook endpoint** for tunnel-based testing (parallel to the CLI): Webhooks → "Add endpoint":
   - `https://vineel-dev-ds9.tail9acde7.ts.net/webhooks/stripe`
   - `https://tom-dev-<host>.tail9acde7.ts.net/webhooks/stripe`
   - Use the per-endpoint signing secret in `STRIPE_WEBHOOK_SECRET`. Either path satisfies the exit criterion.
6. Add Tom as a Developer on the Stripe team.

The actual product/price/meter fixtures are Phase 5 work — **don't create those now**.

### A4. Helicone dev project — non-blocking, lands with Phase 4

**Vineel: Defering this until we need it.**

1. <https://helicone.ai> → sign up; create a workspace/team for Accordli; invite Tom.
2. Project: `accordli-dev` (separate from a future `accordli-staging` and `accordli-prod`).
3. API key into each dev's `.env` as `HELICONE_API_KEY`.

### A5. PostHog dev project — non-blocking, lands with Phase 6

**Vineel: Defering this until we need it.**

1. <https://posthog.com> → US Cloud (per Locked-ish decisions).
2. Project: `accordli-dev`.
3. Project API key (the one used for the SDK ingestion) into `.env` as `POSTHOG_PROJECT_KEY`. Personal API key is *not* needed for ingestion.

### A6. Postmark sandbox — non-blocking, lands with Phase 1

1. <https://postmarkapp.com> → sign up.
2. Create a **Sandbox server** (Postmark's per-server sandbox flag — emails don't actually send, addresses don't get rate-limited).
3. Server token into `.env` as `POSTMARK_SERVER_TOKEN`.
4. Domain verification can wait — sandbox doesn't need it.

### A7. Anthropic API key — already in place

Vineel already has `ANTHRO_API_KEY` set. Phase 4 introduces a second key (Vendor B), so a second Anthropic account/workspace will be needed then. Not a Phase 0 task.

### A8. Account-ownership rollup

Capture this in 1Password (or whatever the password store ends up being) once the dust settles:

| Service       | Account owner | Dev env name      | Shared key   | Per-dev key   |
|---|---|---|---|---|
| Tailscale     | Vineel        | tail9acde7        | —            | machine login |
| WorkOS        | Vineel        | Staging           | api+client   | webhook sig   |
| Stripe        | Vineel        | Test mode         | sk_test, pk_test | wh sig (CLI or endpoint) |
| Helicone (defer) | Vineel        | accordli-dev      | api key      | —             |
| PostHog (defer) | Vineel        | accordli-dev      | project key  | —             |
| Postmark      | Vineel        | sandbox server    | server token | —             |
| Anthropic     | Vineel        | (current key)     | api key      | —             |

---

## Track B — Code changes

Order matters: each step keeps `make dev` green.

1. **db/schema.sql** — collapse `db/migrations/0001`–`0004` into a single file. Drop `db/migrations/`. Drop the `goose` dependency from `go.mod`.
2. **scripts/reset_db.sh** — drop+create the database, apply `db/schema.sql`, leave seed to the existing `seed.sh` invoked by `make reset`.
3. **Makefile** — remove `migrate` and `migrate-down`; rewire `reset` to do schema + seed; add `tunnel`; have `dev` echo the API health URL and tunnel hostname.
4. **Webhook placeholder routes** — `/webhooks/stripe` and `/webhooks/workos`, public (no auth middleware), 200 OK + log byte count. No signature verification yet (Phase 1/5 add real verification).
5. **.env.example rewrite** — every key Phase 0–7 will need, with comments marking which are shared vs per-dev. Real `.env` stays git-ignored.
6. **README rewrite** — brew PG 18, cloudflared, link to this file for the account walkthrough.

---

## Phase 0 exit criterion (re-stated)

Both Vineel and Tom can:

```
git clone …
cp .env.example .env   # paste in the shared + per-dev secrets per A8
make reset             # creates solomocky_dev from schema.sql, seeds Mocky org
make dev               # api on :8080, web on :5173, tunnel hostname echoed
make tunnel            # cloudflared brings up <name>.dev.vinworkbench.com
```

…and trigger a test webhook from either the Stripe dashboard ("Send test webhook") or `stripe trigger payment_intent.succeeded`, or from the WorkOS dashboard's "Send test event", and watch it land at the placeholder route in the API logs.

---

./notes/claude-code-artifacts/phase-0-kickoff.md

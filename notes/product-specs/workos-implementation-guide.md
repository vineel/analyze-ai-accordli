# WorkOS Implementation Guide

This is the spec we implement against. All design decisions are made;
this document captures *what we build*, not *why we picked it*.
Rationale and alternatives live in `notes/research/workos-deepdive.md`.

WorkOS is the identity and authentication substrate for Accordli. It
owns who-can-log-in and the enterprise-identity primitives (SSO, SCIM,
Admin Portal, identity audit log). We own the application data model:
Organizations, Users, Memberships, Departments, Matters, Reviews,
Findings, audit events. WorkOS is **mirrored into our database**, not
authoritative for application data.

---

## 1. Glossary

### WorkOS terms

- **AuthKit** — WorkOS's hosted authentication UX. Email+password,
  Google OAuth, Magic Auth, MFA, passkeys. Reachable as a hosted page
  or as `@workos-inc/authkit-react` components embedded in our app.
- **User** — WorkOS's identity object. Unique by email. May have
  multiple linked auth methods (password, OAuth, SSO, passkey). Has
  `external_id` (≤64 chars) and `metadata` (≤10 string KV pairs).
- **Organization** — WorkOS's grouping of users that share an SSO
  connection or workspace boundary. Has `name`, `domains[]`,
  `external_id`, `metadata`, `stripeCustomerId`.
- **OrganizationMembership** — the join row tying a User to an
  Organization. Carries `role_slug` and `status`
  (`pending` / `active` / `inactive`).
- **Role** — a permission bundle attached to a Membership. Defined
  either at the environment level (defaults applying to all Orgs) or
  Organization level (custom per-customer). Permissions ride in the
  JWT under `permissions[]`.
- **Connection** — one SSO integration with one Organization's IdP
  (SAML or OIDC).
- **Directory** — one SCIM (or HRIS) integration with one
  Organization's directory provider. Pushes user lifecycle events
  in.
- **Admin Portal** — WorkOS-hosted, our-branded self-serve UI for the
  customer's IT admin to configure their Organization's SSO,
  Directory Sync, Domain Verification, and Audit Log streaming.
  Accessed via a one-time setup link we generate from our backend.
- **Domain Verification** — a customer claims ownership of an email
  domain (via DNS record). Once verified, AuthKit can route logins
  from that domain to the customer's SSO connection automatically.
- **Audit Log** — WorkOS's per-Organization event store. Captures
  identity events automatically (sign-in, MFA, SSO config); accepts
  custom events from our app. Streamable to customer SIEMs.
- **Log Stream** — the SIEM-streaming destination configured by the
  customer in the Admin Portal. Six destinations supported.
- **JWT template** — a dashboard-defined customization of access
  token claims. Lets us pull values from `user.metadata` into custom
  JWT claims (e.g. `accordli_dept_id`).
- **JWKS endpoint** — `https://api.workos.com/sso/jwks/<clientId>`.
  Serves the public keys we use to verify access-token signatures.
- **Webhook event** — every state change in WorkOS emits one. We
  subscribe to the events we care about and mirror state into our
  DB.

### Accordli terms (in WorkOS context)

- **Organization** — our top-level customer record. Maps 1:1 to a
  WorkOS Organization. Has its own UUID; the WorkOS id is a foreign
  key. Always exists, even for solos.
- **Department** — a subdivision of an Organization. **Lives in our
  DB only.** WorkOS has no Department primitive. Always exists, even
  for solos (default Dept).
- **User** — our user record. Maps 1:1 to a WorkOS User. Has its own
  UUID; the WorkOS id is a foreign key.
- **Membership** — our row joining User → Organization. Maps 1:1 to
  a WorkOS OrganizationMembership but additionally carries a
  Department FK.
- **`accordli_dept_id`** — custom JWT claim populated by the JWT
  template from `user.metadata.accordli_dept_id`. Read by our Go
  middleware on every authenticated request.
- **`audit_events`** — our application's audit log table. Canonical
  for all audit history. WorkOS audit log is a best-effort mirror.

---

## 2. Architecture

```
┌────────────────────────────────────────────────────────────────┐
│ Accordli (Go API + worker, React frontend)                     │
│                                                                │
│  React frontend                                                │
│   - @workos-inc/authkit-react components for login/signup      │
│   - http-only cookie carries access token                      │
│                                                                │
│  Go API                                                        │
│   - /auth/callback   exchange WorkOS code → tokens             │
│   - JWKS-cached middleware on every authenticated route        │
│   - reads sub, org_id, role, permissions[],                    │
│           accordli_dept_id from validated JWT                  │
│   - mounts request context: { user_id, org_id, dept_id, role } │
│                                                                │
│  Postgres (canonical for application data)                     │
│   - organizations    (mirrors WorkOS Org; FK workos_org_id)    │
│   - users            (mirrors WorkOS User; FK workos_user_id)  │
│   - memberships      (mirrors OrganizationMembership +         │
│                       department_id)                           │
│   - departments      (ours alone)                              │
│   - audit_events     (canonical; dual-written)                 │
│                                                                │
│  Webhook handler                                               │
│   - subscribes to user.*, organization.*,                      │
│     organization_membership.*, session.*, invitation.*,        │
│     connection.activated                                       │
│   - mirrors state into Postgres + audit_events                 │
└──────┬────────────────────────────────────────────┬────────────┘
       │ outbound API (Org/User/Membership CRUD,    │ inbound webhooks
       │ Admin Portal link, audit log emit,         │ (mirror to DB)
       │ token validation)                          │
       ▼                                            │
┌────────────────────────────────────────────────────────────────┐
│ WorkOS                                                         │
│  - Users / Organizations / OrganizationMemberships             │
│  - AuthKit (hosted)                                            │
│  - SSO Connections per customer Org                            │
│  - Directory Sync per customer Org                             │
│  - Audit Log per Org (best-effort mirror of our events;        │
│    plus identity events captured natively)                     │
│  - Admin Portal (launched by setup link)                       │
│  - JWT template: accordli_dept_id ← user.metadata              │
└────────────────────────────────────────────────────────────────┘
```

WorkOS is the system of record for **identity** (who logs in, what
auth methods they have, what SSO connection their Org uses, what
session is active). Accordli is the system of record for **everything
else**, including the application audit history.

---

## 3. The Split: Who Owns What

| Concern | Owner | Notes |
|---|---|---|
| Email + password storage | WorkOS | We never see plaintext or hashes. |
| MFA enrollment / verification | WorkOS | Configured per Org policy. |
| Session minting + JWT signing | WorkOS | We validate; we don't mint. |
| Session revocation | WorkOS | We call `/logout` to revoke. |
| SSO / SAML / OIDC connections | WorkOS | Per customer Org. |
| SCIM ingest (lifecycle from customer IdP) | WorkOS | Webhooks to us. |
| Identity audit events (sign-in, MFA, etc.) | WorkOS | Mirrored into our `audit_events` via webhook. |
| Customer-facing SSO setup UI | WorkOS (Admin Portal) | We just generate the link. |
| Application data (Matter, Review, Finding) | Accordli | Never goes near WorkOS. |
| Department model | Accordli | WorkOS has no Department concept. |
| Application audit events (matter created, review run, …) | Accordli | Dual-written: our DB canonical, WorkOS best-effort. |
| Stripe Customer + Subscription state | Accordli | WorkOS holds only `stripeCustomerId` reference. |
| Plan / entitlement enforcement | Accordli | Our `usage_events` + `credit_ledger`. |
| RBAC enforcement on API routes | Accordli | We read `permissions[]` from validated JWT. |
| FGA-style fine-grained checks | Accordli | Postgres queries with `org_id` + `dept_id`. No FGA product in v1. |

---

## 4. Data Model Mirror

```
Accordli DB                          WorkOS
───────────                          ──────
organizations                  ←→    Organization
  id (uuid)                            id (org_xxx)
  workos_org_id (FK)                   external_id ← our uuid
  name                                 name
  metadata jsonb                       metadata (≤10 KV; we use it
                                                  sparingly: cache
                                                  is_solo, etc.)
  stripe_customer_id            ←→     stripeCustomerId
  tier                                 (in metadata)
  is_solo                              metadata.is_solo

users                          ←→    User
  id (uuid)                            id (user_xxx)
  workos_user_id (FK)                  external_id ← our uuid
  email                                email
  current_dept_id (FK)          ←→     metadata.accordli_dept_id
                                          (the JWT-template source)

memberships                    ←→    OrganizationMembership
  id (uuid)                            id (om_xxx)
  workos_membership_id (FK)
  user_id (ours)
  organization_id (ours)
  department_id (FK)  ← OURS           role_slug
  role  (mirrored from WorkOS)         status

departments                          [no native counterpart]
  id (uuid)
  organization_id
  name

JWT (issued by WorkOS):
  sub, sid, iss, org_id, role, permissions[], exp, iat,
  accordli_dept_id   ← from JWT template
```

**Invariant:** `users.current_dept_id` and
`user.metadata.accordli_dept_id` in WorkOS are kept in sync. Updating
one without the other is a bug; do it through the
`syncDeptToWorkOS()` helper.

---

## 5. Linear Integration Flow

The order in which we wire things up. Each step is concrete and
testable; downstream steps assume earlier ones are in place.

### Step 1 — WorkOS project setup (one-time, manual)

- Create two WorkOS environments: **Staging** and **Production**.
- Enable AuthKit. Allow email+password and Google OAuth at minimum.
- Configure MFA: optional for solo, required-for-admin role on team
  plans. (Default policy can be tightened per-Org later.)
- Define environment-level roles: `owner`, `admin`, `member`. Map
  permissions per role (least privilege).
- Configure a JWT template that adds the custom claim:
  - claim name: `accordli_dept_id`
  - source: `user.metadata.accordli_dept_id`
- Configure webhooks pointing at our `/webhooks/workos` endpoint.
  Subscribe to:
  - `user.*`
  - `organization.*`
  - `organization_membership.*`
  - `session.created`, `session.revoked`
  - `invitation.*`
  - `connection.activated`, `connection.deactivated`
  - `dsync.*` (Directory Sync events)
- Custom domain: `auth.accordli.com` (paid feature; configure DNS).
- Set up the Stripe add-on link in dashboard if available; otherwise
  we set `stripeCustomerId` via API.

Snapshot the JWT template config and webhook subscription list in
`infra/workos-config.md` for repo-side documentation. Not IaC; a
person sets this up, the doc keeps it from being load-bearing-secret.

### Step 2 — Define our DB schema

```sql
-- pseudocode

create table organizations (
  id              uuid primary key default gen_random_uuid(),
  workos_org_id   text unique not null,
  name            text not null,
  tier            text not null,         -- 'solo' | 'team' | 'enterprise'
  is_solo         boolean not null default true,
  stripe_customer_id text unique,
  metadata        jsonb not null default '{}',
  created_at      timestamptz not null default now(),
  ...
);

create table departments (
  id              uuid primary key default gen_random_uuid(),
  organization_id uuid not null references organizations,
  name            text not null,
  is_default      boolean not null default false,
  created_at      timestamptz not null default now()
);

create table users (
  id              uuid primary key default gen_random_uuid(),
  workos_user_id  text unique not null,
  email           text unique not null,
  current_dept_id uuid references departments,  -- single-org-per-user model
  created_at      timestamptz not null default now()
);

create table memberships (
  id                    uuid primary key default gen_random_uuid(),
  workos_membership_id  text unique not null,
  user_id               uuid not null references users,
  organization_id       uuid not null references organizations,
  department_id         uuid not null references departments,
  role                  text not null,        -- mirrored from role_slug
  status                text not null,        -- 'active' | 'pending' | 'inactive'
  created_at            timestamptz not null default now(),
  unique (user_id, organization_id)
);

create table audit_events (
  id              uuid primary key default gen_random_uuid(),
  organization_id uuid not null references organizations,
  user_id         uuid references users,        -- nullable for system actors
  action          text not null,
  target_type     text,
  target_id       text,
  metadata        jsonb not null default '{}',
  occurred_at     timestamptz not null default now(),
  source          text not null                  -- 'app' | 'workos' | 'system'
);
```

Postgres RLS policies on `matters`, `reviews`, `findings`, etc.,
filter by `organization_id` (and `department_id` where applicable).
Tenant isolation defense-in-depth.

### Step 3 — Login flow (frontend → backend)

```
frontend                             backend                       WorkOS
────────                             ───────                       ──────
[user] click "Log in"
  │
  └── render <SignIn /> from @workos-inc/authkit-react
         │
         └─[user submits email/password / Google / SSO]──► AuthKit
                                                              │
                                                              │ on success,
                                                              │ POST code to
                                                              │ our callback
              ◄──────────────[/auth/callback?code=xxx]────────┘
[server]
  │
  ├── exchange code:
  │     tokens = workos.userManagement
  │       .authenticateWithCode({ code })
  │     // returns { access_token, refresh_token, user, ... }
  │
  ├── lookup or create our user row by workos_user_id
  │     (mirroring is also done by webhook; lookup-or-upsert here
  │     guarantees the user is present even if webhook is delayed)
  │
  ├── pick the org context:
  │     if user has 1 active membership:
  │       org_id = that org
  │     else:
  │       redirect to /select-org
  │
  ├── re-authenticate into the org:
  │     tokens = workos.userManagement
  │       .authenticateWithRefreshToken({
  │         refresh_token: tokens.refresh_token,
  │         organization_id: org_id })
  │     // new access_token now contains org_id, role,
  │     // permissions[], accordli_dept_id
  │
  ├── set http-only cookie:
  │     cookie("accordli_session", access_token,
  │            secure=true, samesite=lax)
  │     refresh_token stored server-side keyed by sid
  │
  └── redirect to /dashboard
```

### Step 4 — Backend session validation

```go
// pseudocode

middleware Authenticate(req) {
    raw := cookie(req, "accordli_session")
    claims, err := jwt.ValidateWithJWKS(raw, JWKS_URL, jwksCache)
    if err { return 401 }

    ctx := req.Context
    ctx.Set("user_id_workos", claims.sub)
    ctx.Set("org_id_workos", claims.org_id)
    ctx.Set("session_id", claims.sid)
    ctx.Set("role", claims.role)
    ctx.Set("permissions", claims.permissions)
    ctx.Set("dept_id", claims.accordli_dept_id)

    // Optionally lookup our local user/org rows by workos id
    // and cache for the request. Cheap if memoized per-request.

    return next(req)
}
```

JWKS is fetched once and cached for ~1 hour with refresh-on-kid-miss.
Cache is a process-local in-memory store.

### Step 5 — Webhook handler (mirror state into our DB)

```
POST /webhooks/workos

[server]
  │
  ├── verify signature (WorkOS signs every webhook)
  │
  ├── dedupe on event.id (insert-or-skip in webhook_events table)
  │
  ├── switch event.type:
  │     "user.created":         upsert into users
  │     "user.updated":         update users (email, etc.)
  │     "user.deleted":         soft-delete in users
  │
  │     "organization.created": upsert into organizations
  │     "organization.updated": update organizations
  │     "organization.deleted": soft-delete organizations
  │
  │     "organization_membership.created":
  │         upsert into memberships
  │         derive department_id: pull from invite metadata
  │           if present, else default to org's default Dept
  │         write audit_event "membership.created"
  │
  │     "organization_membership.updated":
  │         update role / status
  │         write audit_event "membership.updated"
  │
  │     "organization_membership.deleted":
  │         status = 'inactive' (soft-delete)
  │         write audit_event "membership.deleted"
  │
  │     "session.created":
  │         write audit_event "user.signed_in"
  │     "session.revoked":
  │         write audit_event "user.signed_out"
  │
  │     "invitation.*":
  │         write audit_event mirroring action
  │
  │     "connection.activated":
  │         flag org as SSO-enabled
  │         write audit_event "sso.connection_activated"
  │
  │     "dsync.user.created" / "dsync.user.updated" / ".deleted":
  │         provision/deprovision via membership upsert/delete
  │         write audit_event mirroring action
  │
  └── 200 OK
```

**All webhook handling is idempotent.** Every event has an id; every
mirror is upsert-shaped or insert-or-skip. Re-delivery is harmless.

### Step 6 — Signup flow (solo path with Stripe)

```
1. /signup?plan=pro → render <SignUp /> from authkit-react
2. user creates account → AuthKit posts back to /auth/callback?code
3. /auth/callback exchanges code → tokens (no org_id yet)
4. detect signup flow: token has no org_id
5. transaction:
     a. workos.organizations.create({
          name: defaultName(user.email),
          external_id: our_org_uuid,
          metadata: { is_solo: "true" }
        })
     b. our DB: insert organizations, departments (default), users
     c. workos.organizationMemberships.create({
          user_id, organization_id, role_slug: "owner"
        })
     d. our DB: insert memberships, set users.current_dept_id
     e. workos.users.update(user_id, {
          metadata: { accordli_dept_id: our_default_dept_id }
        })
     f. stripe.customers.create({
          email, metadata: { workos_org_id, accordli_org_id }
        })
     g. our DB: organizations.stripe_customer_id = stripe_customer.id
     h. workos.organizations.update(workos_org_id, {
          stripeCustomerId: stripe_customer.id
        })
6. re-authenticate into org:
     workos.userManagement.authenticateWithRefreshToken({
       refresh_token, organization_id
     })
     → new JWT carries org_id, role, accordli_dept_id
7. redirect to Stripe Checkout for the chosen plan
8. Stripe webhook on subscription.created →
     update organizations.tier, billing_periods, etc.
9. redirect to /dashboard
```

**Idempotency.** Every external call uses an idempotency key derived
from the user's signup attempt id. If step 5e fails, retrying the
whole flow is safe. A separate cleanup cron tears down half-built
WorkOS Orgs older than N hours that have no successful Stripe
subscription.

### Step 7 — Invite flow (team plans)

Pre-condition: org has `tier = "team"` and `is_solo = false`.

```
1. Org admin enters teammate email + chosen role + chosen department
2. server creates an invitation:
     workos.invitations.create({
       email, organization_id, role_slug,
       expires_in_days: 14
     })
   Pass our department_id in invite metadata (or store our-side keyed
   by invitation id).
3. WorkOS sends the invitation email
4. invitee clicks accept → AuthKit signup or login flow
5. webhook: organization_membership.created
   handler reads our department_id from invite metadata
   inserts memberships with that department_id
   updates user.metadata.accordli_dept_id in WorkOS
6. invitee lands on /dashboard, dept context populated in JWT
```

### Step 8 — SSO setup flow (Admin Portal)

For Large Team or Enterprise customers turning on SSO.

```
1. Accordli sales/CS rep (in our admin tool) clicks
   "Generate SSO setup link" for a customer org.
2. server:
     link = workos.portal.generateLink({
       organization: workos_org_id,
       intent: "sso",
       return_url: "https://app.accordli.com/admin/sso-setup-done"
     })
3. server emails the link to the customer's IT admin
   (or copies to clipboard and Accordli rep sends it manually).
4. customer IT admin clicks link → lands in WorkOS-hosted,
   Accordli-branded Admin Portal.
   - configures their IdP (SAML or OIDC) per the wizard
   - completes Domain Verification by adding DNS records
   - clicks Activate
5. webhook: connection.activated → server marks org SSO-enabled
6. first SSO login attempt:
   user enters alice@lawfirm.com → AuthKit sees verified domain →
   redirects to customer IdP → user authenticates → assertion posted
   to WorkOS ACS → JWT minted → /auth/callback as usual
7. if SCIM is also configured, dsync.* webhooks provision/deprovision
   memberships automatically
```

### Step 9 — Audit logging (dual-write)

Single helper:

```go
// pseudocode

func emitAudit(ctx, event AuditEvent) error {
    // 1. Canonical write to our DB (synchronous, must succeed)
    if err := db.InsertAuditEvent(ctx, event); err != nil {
        return err
    }
    // 2. Best-effort write to WorkOS (async, log failure but don't raise)
    go func() {
        err := workos.AuditLogs.CreateEvent(event.OrgWorkOSID, {
          action: event.Action,
          actor: { type: "user", id: event.UserWorkOSID },
          targets: event.Targets,
          occurred_at: event.OccurredAt,
        })
        if err != nil { logWarn("workos audit emit failed", err) }
    }()
    return nil
}
```

Call sites: any meaningful action — Matter created, Review run,
finding exported, settings changed, member invited, plan changed,
admin impersonation, etc.

Identity events (sign-in, MFA, SSO config) are **not** emitted by us
— WorkOS captures them natively, the webhook handler mirrors them
into our `audit_events` with `source = 'workos'`.

### Step 10 — Signout flow

```
1. user clicks Sign Out → POST /auth/signout
2. server reads sid from validated JWT
3. server calls workos.userManagement.logout({ session_id: sid })
4. server clears the access_token cookie and the refresh-token store
5. server emits audit "user.signed_out"
   (also gets one from WorkOS via session.revoked webhook;
    dedupe on session_id + occurred_at if both arrive)
6. redirect to /
```

### Step 11 — Cancellation and account deletion

**Soft cancel (subscription cancelled, account stays for grace
period):**

```
1. user clicks Cancel in /account → Stripe API: cancel_at_period_end = true
2. emit audit "subscription.cancellation_scheduled"
3. ... at period end, Stripe webhook fires customer.subscription.deleted
4. server: organizations.tier = "cancelled"
   - revoke API access (or downgrade to read-only)
   - emit audit "subscription.terminated"
   - schedule data-retention job (deletion in 30/60 days)
5. WorkOS state untouched: org and memberships stay active so the
   user can log in to view their data during the grace period
```

**Hard delete (GDPR / explicit user request):**

```
1. user requests account deletion in /account
2. confirm via emailed link (24-hour window, audit-logged)
3. deletion job (transactional where possible):
     a. delete app data: matters, reviews, findings (cascade)
     b. delete blob storage objects for the org
     c. workos.organizationMemberships.delete(...) for each member
     d. workos.users.delete(user_id)  -- if no remaining memberships
     e. workos.organizations.delete(org_id)  -- if last user out
     f. emit final "account.deleted" event from system actor
4. retain anonymized billing/usage rows per retention policy
```

---

## 6. Promote-Org Path (solo → team)

Trigger: solo clicks "Invite a teammate" or "Upgrade to Small Team."

```
1. require plan upgrade if currently solo:
     a. Stripe API: update subscription Pro/Gold → Small Team Price
     b. on Stripe webhook subscription.updated:
        organizations.tier = "team"
        organizations.is_solo = false
2. prompt user to set real Org name (replaces "alice@…'s workspace")
   and real Department name (replaces "Default")
   - workos.organizations.update(workos_org_id, { name: new_name })
   - departments.update(default_dept_id, { name: new_name })
3. team UX (members panel, invite form, dept admin) becomes visible
4. invite the new teammate via Step 7 flow
```

**Invariants preserved.** Organization id, User id, Department id,
Stripe Customer id all unchanged. All Matters, Reviews, Findings,
audit events stay attached. Same WorkOS Organization, same Stripe
Subscription (modified, not replaced).

---

## 7. Department Sync Helper

Single point of mutation for `users.current_dept_id` and the
WorkOS-side mirror.

```go
// pseudocode

func syncDeptToWorkOS(ctx, userID, newDeptID) error {
    user := db.GetUser(ctx, userID)
    if user.current_dept_id == newDeptID { return nil }   // no-op

    // 1. Update our DB first (canonical)
    db.UpdateUser(ctx, userID, { current_dept_id: newDeptID })

    // 2. Mirror to WorkOS user metadata
    workos.users.update(user.workos_user_id, {
        metadata: { accordli_dept_id: newDeptID.String() }
    })

    // 3. Force a refresh of the user's active sessions so the new
    //    JWT carries the updated claim. Pseudo: enumerate sessions
    //    via session_id index and revoke; user re-auths on next
    //    request and gets a fresh token via refresh_token flow.
    db.MarkSessionsForRefresh(ctx, userID)

    // 4. Audit
    emitAudit(ctx, AuditEvent{
        Action: "user.department_changed",
        UserID: userID,
        Targets: [{ type: "department", id: newDeptID }],
    })

    return nil
}
```

Call sites: signup (initial dept set), invite acceptance (dept from
invite metadata), promote-org (default dept rename does not change
id, no sync needed), admin-driven dept reassignment.

---

## 8. Operational Surface

- **JWKS cache.** Process-local, ~1 hour TTL, refresh on `kid` miss.
  Means brief WorkOS API outages don't break already-issued sessions.
- **Webhook retry.** WorkOS retries delivery on non-2xx with
  exponential backoff. Our handler is idempotent; storms are safe.
- **Cleanup cron.** Daily sweep for half-built WorkOS Orgs (created
  but no Stripe subscription within N hours) → tear down.
- **Health check.** `/health/workos` issues a cheap API call (e.g.
  `workos.organizations.get(known_test_org_id)`) and surfaces status
  on our internal dashboard.

---

## 9. What's Deferred (post-MVP)

- **Multi-Org users.** A single human in multiple Accordli Orgs.
  Skipped for v1 per `notes/research/workos-deepdive.md` Phase 1
  question 3 (Tom: "we don't care about this right now"). When
  needed, the JWT-template approach for `accordli_dept_id` becomes
  ambiguous; switch to per-request DB lookup of dept (memberships
  table) at that point.
- **"Log out everywhere."** WorkOS does not expose this. Build
  ourselves by tracking active `sid`s per user and fanning out
  `/logout` calls. Not v1.
- **WorkOS Audit Log retention extension.** $99/mo per 1M events for
  retention beyond default. Skip until an enterprise customer needs
  WorkOS-dashboard view for >30 days or SIEM backfill.
- **WorkOS Audit Log SIEM streaming.** $125/mo per stream. Wire when
  the first enterprise customer asks; cheaper than building Datadog
  / Splunk integrations ourselves.
- **FGA.** Skipped. Plain `dept_id` checks in API middleware are
  enough until question 10b in `todo.md` becomes real.
- **Vault.** Skipped. We use Azure Key Vault directly for secrets.
- **Radar.** Skipped. Add when bot/fraud signals warrant.
- **Custom domain branding deeper than `auth.accordli.com`.** Default
  is fine for v1.

---

## 10. References

- Decision rationale: `notes/research/workos-deepdive.md`
- Pricing projections by scenario: `notes/research/workos-deepdive.md` §Phase 4
- Audit log boundary: `notes/research/workos-deepdive.md` §Phase 6
- Alternatives (Entra External ID): `notes/research/workos-deepdive.md` §Phase 13
- Stripe orchestration spec: `notes/product-specs/stripe-implementation-guide.md`
- Open research questions: `notes/todo.md` (item 15)

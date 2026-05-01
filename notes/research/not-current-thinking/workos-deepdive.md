# WorkOS Deep Dive — Implementing Auth and Identity for Accordli

A focused design document for how Accordli will use WorkOS as its identity and access layer. Builds on the broader vendor evaluation in `../../../workbench-accordli/notes/auth-research-workos.md` (sibling repo, dated 2026-04-27); this file pivots from "should we pick WorkOS" to "given that we have, how do we wire it into the stack we have."

WorkOS as the auth/identity choice is locked-ish per `CLAUDE.md`. The questions this document answers are:

1. Which WorkOS products do we adopt, when, and at what cost?
2. How does Accordli's `Organization → Department → User` model map onto WorkOS primitives?
3. What do we get out of the box and what must we build?
4. Where does the audit-log boundary live between WorkOS-recorded events and our own `audit_events` table?
5. What does the runtime architecture look like, drawn end-to-end against the rest of the stack?
6. What's the implementation order?

Prices and product boundaries come from public sources current as of 2026-04-27. Re-verify before signing.

Resolves `notes/todo.md` item 15.

---

## 1. Stack context

The decisions WorkOS plugs into, restated only enough to ground what follows:

- **Backend.** Go API and Go River worker, both running in Azure Container Apps.
- **Frontend.** React + TypeScript on Azure Static Web Apps.
- **Database.** Azure Postgres Flexible Server. River queue lives in the same DB.
- **Object storage.** Azure Blob (Hot, ZRS) — contracts, parsed markdown, generated reports.
- **LLM.** Claude via Azure AI Foundry (Vendor A) with direct Anthropic as Vendor B failover.
- **Billing/metering.** Stripe-only for MVP and Phase 1 (no Orb, no Lago). Stripe owns subscriptions, invoices, dunning, tax, and Credit Grants; Accordli owns real-time entitlement, the immutable ledger, and reconciliation. Full spec in `../product-specs/stripe-implementation-guide.md`. WorkOS's Stripe glue still does not earn its place — see §11.
- **Observability.** Sentry + Grafana Cloud (OTel) + BetterStack + PostHog + Instatus.

Customer mix, ordered by likelihood for the first ~12 months: solo practitioners → small firms (3–10 lawyers, no SSO) → mid-market firms with SSO → enterprise law firms with SSO + SCIM + audit-log export demands.

---

## 2. Scope

This document covers:

- WorkOS product surface area, mapped to which Accordli use case lights it up
- Identity model bridge between Accordli and WorkOS
- Authentication flow, session validation, and webhook handling
- The authorization model (built-in roles, app-level rules, FGA deferral)
- Audit log architecture and the WorkOS/`audit_events` boundary
- Phased rollout

Out of scope:

- WorkOS vs. competitor selection — settled
- Detailed WorkOS API call examples — implementation-time concern
- The Department/billing tension (`todo.md` item 10) — orthogonal to auth
- Billing mechanics — covered in `../product-specs/stripe-implementation-guide.md`. This document only covers the seam between WorkOS and that system.

---

## 3. WorkOS product surface

WorkOS is a suite, not a product. Below: what each piece does, when we adopt it, and what it costs at our scale.

| Product | What it covers | When we adopt | Cost at our scale |
|---|---|---|---|
| **AuthKit** | Hosted login/signup UI, email+password, magic links, social (Google, Microsoft, Apple, GitHub), MFA (TOTP + SMS), passkeys, sessions, password reset, JWT issuance | Day one | Free up to 1M MAU; $2,500/M-MAU above. Effectively free for years. |
| **Organizations** | First-class tenants, member roster, invites, roles | Day one | Free, unlimited. |
| **SSO (SAML/OIDC)** | Per-firm SAML or OIDC connection to Okta/Entra/Google Workspace/etc. | First firm customer who asks | $125/connection/month |
| **Directory Sync (SCIM)** | IdP-driven auto-provisioning and de-provisioning | First enterprise customer | $125/connection/month (often bundled with SSO in negotiation) |
| **Audit Logs API** | Append-only log of identity events; export to JSON/CSV | Day one (free with AuthKit) | Free with AuthKit; verify retention default and export model |
| **Admin Portal** | White-label IT-admin self-serve UI for SSO/SCIM/branding | When we ship the firm tier | Bundled with SSO/SCIM |
| **FGA** | Zanzibar-style relationship-based authorization | Deferred — see §10 | Priced separately; revisit if/when matter-level access rules appear |
| **Radar** | Anti-fraud (geo-blocking, suspicious-signup challenges) | Defer until self-serve abuse appears | Priced separately |

We do **not** plan to use:

- **WorkOS's Stripe Entitlements integration.** We do have a direct Stripe relationship now (no Orb, no Lago), but our entitlement gate runs against the local `credit_ledger` mirror — not against a JWT claim derived from Stripe subscription state. See §11.
- **WorkOS's Stripe Seat Sync.** Our plans are flat fees with a fixed app-enforced seat limit per plan, not per-seat metering. Seat Sync solves a problem we don't have. See §11.
- **Connect / MCP / Pipes.** Adjacent products with no current Accordli use case.

**Realistic monthly WorkOS line item:** $0 for the first ~6 months. $125–$250 once the first SSO firm signs. $250–$500 by month 18 with two SSO firms and one SCIM directory. Matches the budget already booked in `azure-proposal.md` §4.3.

---

## 4. Identity model bridge

The friction point: Accordli has a three-level model (`Organization → Department → User`). WorkOS has two (`Organization → User`). Bridging cleanly matters because the wrong choice here calcifies fast.

### 4.1 Organization → WorkOS Organization (1:1)

Each Accordli Organization corresponds to exactly one WorkOS Organization. The WorkOS `organization_id` is the foreign key on our `organizations` table; we never let two Accordli Orgs share one WorkOS Org or vice versa.

A solo practitioner is a one-person Organization. We considered modeling solos as standalone users without an Organization (cheaper conceptually), but the cost of branching every authorization check on "is this a solo or a firm?" exceeds the cost of carrying a one-row Org per solo. The Organization-always invariant is worth defending.

### 4.2 Department lives in our DB only

WorkOS does not have a Department primitive. We do not try to invent one in WorkOS (e.g., by abusing groups or roles). Departments are an Accordli domain concept tied to Matter ownership and team-plan ARC pooling — not an identity concept. They live exclusively in our `departments` table.

When firms eventually want IdP-group-driven Department auto-assignment (an enterprise feature, not a launch one), the integration goes one of two ways:

1. SCIM group-to-Department mapping handled in our app: WorkOS Directory Sync delivers SCIM groups; our webhook handler maps `{idp_group_name → department_id}` per Org and updates `users.department_id` accordingly.
2. WorkOS-side custom user attributes carrying a Department label that the firm's IdP populates.

Either is feasible later; neither is needed at launch.

### 4.3 User → WorkOS User (1:1)

WorkOS owns the canonical identity record. Our `users` table mirrors a minimal subset:

```
users (
  id              uuid primary key,             -- our id, used in domain FKs
  workos_user_id  text not null unique,         -- the bridge
  organization_id uuid not null references organizations(id),
  department_id   uuid not null references departments(id),
  email           text not null,                -- mirrored from WorkOS for display, not auth
  created_at      timestamptz,
  ...
)
```

We never store passwords, MFA secrets, OAuth tokens, or SSO assertions. WorkOS owns those.

The mirror exists for three reasons: foreign keys to domain rows (a Matter belongs to a User), tenant-scoped queries (`SELECT ... WHERE organization_id = $1`), and Postgres RLS policies (which need the org id locally to evaluate). It is rebuildable from WorkOS at any time.

### 4.4 The "one User, exactly one Org" constraint

WorkOS allows a User to be a member of multiple Organizations. Accordli's glossary says a User belongs to **exactly one** Organization. We enforce the Accordli constraint, not WorkOS's permissiveness.

Concretely: at signup we create a fresh User row tied to one Org. If the same human (same email) joins a second Org, that's a second User row with a second `workos_user_id`, even if WorkOS happens to expose this as one identity with two memberships. The duplication is acceptable; the semantic clarity isn't negotiable.

A consequence: when a lawyer leaves Firm A and joins Firm B with the same email, the human flow is "Firm A revokes; Firm B invites; the user re-authenticates." We do not migrate the user record across orgs.

If this turns out to bite often (e.g., contract lawyers floating between firms), we revisit. Not a launch problem.

---

## 5. What WorkOS gives us out of the box

This is the section to reread when sizing scope. For each capability: what WorkOS does, where the gap is.

### 5.1 AuthKit (login UX, sessions, MFA, social, magic links)

WorkOS handles: hosted login page, hosted signup page, password reset, MFA enrollment and challenge, magic link emails, social provider OAuth dances, passkey/WebAuthn registration, session token issuance and refresh, account-recovery flows.

**Gap:** the post-login UX is ours. After WorkOS hands back a session, our React app routes to the right org dashboard, the right Department's matters, the right plan-tier features. None of that is auth.

**Branding gap:** the AuthKit hosted UI is configurable but not pixel-perfect. For solo practitioners and small firms, it's fine. For firms sensitive to white-labeling, the Admin Portal covers it. For Accordli's own login look-and-feel, we accept WorkOS's branding controls and skin them; we don't self-host the login page.

### 5.2 Organizations

WorkOS handles: org creation, member invite emails, member acceptance, role assignment within an Org, org metadata, hosted "manage your org" UI.

**Gap:** we still mirror Org rows in our DB for FK and RLS reasons (§4.3). We still write the Account Page UI inside Accordli — WorkOS's "manage org" hosted UI is for IT admins configuring SSO, not for an Org admin reviewing their plan, ARC balance, billing history, etc.

### 5.3 SSO (SAML/OIDC)

WorkOS handles: per-Org SAML 2.0 or OIDC connection, hosted IdP discovery, JIT user provisioning on first sign-in, the actual SAML/OIDC handshake with the firm's IdP (Okta, Entra, Google Workspace, OneLogin, Ping, JumpCloud), normalized profile output.

**Gap:** none, really. SSO is the closest thing to "free out of the box" in this entire matrix — for $125/Org/month, the firm's IT admin and our backend both get clean APIs and a hosted config flow.

The thing we have to remember to do is bill the customer for it. SSO is an Enterprise-tier feature in our pricing, and the $125/Org cost passes through. Don't quietly absorb it.

### 5.4 Directory Sync (SCIM)

WorkOS handles: SCIM 2.0 endpoint per Org, push-based provisioning from the firm's IdP, deactivation events, group sync (groups and group memberships flow through as events on a webhook).

**Gap:** the action our app takes on each SCIM event is ours to design and write. A `user.created` webhook from Directory Sync means "create a User in our DB, assign to the right Department, default role." That's a small but non-trivial bit of glue per supported event.

### 5.5 Audit Logs API

WorkOS handles: append-only log of identity events (auth, session, org membership, role change, SSO connection config change, SCIM event, Admin Portal action), filterable by Org, exportable on demand.

**Gap:** WorkOS only logs what WorkOS sees. The full SOC 2 audit story requires our own `audit_events` table for domain events. The boundary between the two is the entire content of §7.

### 5.6 Admin Portal (hosted IT-admin UI)

WorkOS handles: a per-Org URL we hand to a firm's IT admin where they can configure their SSO connection, SCIM directory, allowed domains, and (optionally) brand the login page, all without an engineering ticket to us.

**Gap:** none for the IT-admin path. We do still need our own Org admin UI for the customer-facing concerns (plan, billing, members visible inside Accordli, audit log review).

### 5.7 FGA — explicitly deferred

WorkOS FGA is a Zanzibar-shaped permissioning service for object-level rules ("user X can read matter Y if X is on team Z"). Tempting on paper.

In practice, our launch authorization model is two layers:

1. **Org boundary** — every domain row carries `organization_id`; users can only act on rows in their own Org. Enforced via Postgres RLS plus app-level checks. This handles ~80% of access concerns.
2. **Department + role** — `users.department_id` plus a small role enum (`org_admin`, `member`, possibly `viewer` for guest counsel). Enforced in app code per route.

That covers the matter-access cases we anticipate for solo, small firm, and even mid-market firm tiers. FGA enters the picture if and only if a firm wants partner/associate/paralegal-shaped fine-grained matter visibility ("Alice can see matters tagged 'IP'; Bob can see only matters where he's listed as a contributor"). When that happens, FGA replaces the per-route checks; the Org-boundary RLS layer stays.

Don't pre-build for FGA. Don't model relationship tuples in our schema today. Add FGA as a layer when it earns its place.

### 5.8 Radar (anti-fraud)

WorkOS Radar blocks signup abuse and lets you geo-block or SMS-challenge suspicious sign-ups.

We defer Radar. Solo practitioner signups will be sparse enough that abuse is hand-reviewable; the refund-policy abuse vector (`todo.md` item 13) is a separate concern Radar doesn't help with. Adopt Radar when self-serve signup volume grows enough that anomaly detection beats human review — likely month 9+.

---

## 6. What we still build

A list, ordered roughly by the order in which the work surfaces during implementation.

1. **Mirror schema.** `organizations`, `departments`, `users` tables with WorkOS id columns and FKs.
2. **Sign-in / sign-up redirect handlers.** API routes that initiate the WorkOS flow and consume the callback.
3. **Session middleware.** Go middleware that takes the WorkOS session token from the cookie, validates it, looks up local user/org context, and attaches it to the request.
4. **Postgres RLS policies.** `USING (organization_id = current_setting('app.current_org')::uuid)` on every tenant-scoped table; the session middleware sets the GUC at request start.
5. **WorkOS webhook handler.** Receives `user.created`, `user.updated`, `user.deleted`, `organization.created`, `organization.updated`, `organization.deleted`, `organization_membership.*`, plus SCIM events when in scope. Mirrors them to our DB. Idempotent on event id.
6. **React route guard.** `<RequireAuth>` wrapper that triggers the WorkOS redirect if no session; loads the user/org context into a React context provider at the top of the tree.
7. **Account / org admin UI.** Plan, members, ARC balance, audit log view, credit pack purchases. None of this is in WorkOS's hosted Org UI.
8. **Domain audit log.** `audit_events` table and write-points throughout the app. See §7.
9. **Org → Stripe Customer mapping.** Stripe Customer creation happens inline in the signup flow per `stripe-implementation-guide.md` §5, not from the WorkOS webhook. The WorkOS webhook only writes the local Org row; Stripe Customer creation belongs to the API code path that runs Embedded Checkout. See §11 for why these are separate.
10. **Department concept.** Tables, default-Department-per-Org for solos, admin UI for firms to create/manage Departments.
11. **App-level authorization checks.** Per-route role checks; matter ownership checks; plan-tier feature gating.
12. **Compliance view.** Admin tooling that joins WorkOS Audit Logs API output with our `audit_events` for SOC 2 evidence and customer audit-log exports. See §7.

That list is shorter than it would be without WorkOS. Without WorkOS, items 1, 2, 3, 5, and 6 each grow significantly, and we add: password storage and rotation, MFA enrollment and challenge, social OAuth flows, SAML SP implementation, SCIM endpoint, hosted IT-admin UI, account recovery, session refresh logic. WorkOS earns its line item by deleting all of that.

---

## 7. The audit-log boundary

Of every architectural decision in this document, this is the one most likely to bite us if we get it wrong, because SOC 2 evidence and customer audit-log requests both depend on it.

### 7.1 Principle

Three rules, applied in this order:

1. **Identity events live in WorkOS only.** Authentication, session lifecycle, MFA challenge, password operations, SSO/SCIM operations, Admin Portal actions. We have no information to add; mirroring would only invite drift between two sources of truth.
2. **Domain events live in `audit_events` only.** Matter created, contract uploaded, ReviewRun started/completed, Finding accepted/rejected, plan changed, ARC reserved/committed/rolled back, credit pack purchased, refund issued, retention policy changed, data exported. WorkOS has no visibility into these and never will.
3. **Cross-cutting events are double-recorded.** A small set lives in both. The duplication is deliberate, scoped to events that change either authorization decisions or billing-relevant state.

### 7.2 Cross-cutting events (double-recorded)

| Event | Why also in `audit_events` |
|---|---|
| Session established | Local trail tying a WorkOS session id to subsequent app actions; survives a WorkOS read-side outage during a compliance request |
| Org member added | Changes seat count for billing; changes our authorization scope |
| Org member removed | Same |
| Role changed | Changes app-level authorization; we want a local record without depending on WorkOS retrieval |
| SSO connection added/removed (per Org) | Materially changes the security posture of the Org; surfaces in customer trust pages and questionnaires |

Each of these gets a row in `audit_events` written by our webhook handler. The row carries `workos_event_id` for traceback, plus our normalized fields.

### 7.3 What `audit_events` looks like

Append-only table. Schema sketch (not the spec):

```
audit_events (
  id                   uuid pk,
  organization_id      uuid not null,
  actor_user_id        uuid,                   -- our user id, may be null for system events
  actor_workos_user_id text,                   -- denormalized for ease of join
  event_type           text not null,          -- e.g. 'matter.created', 'session.established'
  resource_type        text,                   -- 'matter', 'review', 'plan'
  resource_id          uuid,
  payload              jsonb,                  -- event-specific structured detail
  workos_event_id      text,                   -- present for cross-cutting double-recorded events
  ip_address           inet,
  user_agent           text,
  occurred_at          timestamptz not null
);
create index on audit_events (organization_id, occurred_at desc);
```

Notes:

- Append-only enforced at the application layer (no `UPDATE` or `DELETE` paths in code) and reinforced at the DB layer (revoke `UPDATE`/`DELETE` from the app role).
- `payload` JSONB is for event-specific detail. Resist the temptation to define a column for every field a future event might need; the ergonomics of evolving an audit schema with rigid columns are bad.
- `organization_id` is on every row. Customer audit-log exports filter by it.

### 7.4 Joining the two systems

Two operational scenarios:

**Customer asks for their audit log.** ("Show us everything that happened in our Org last quarter.") We hit two sources:

1. WorkOS Audit Logs API, filtered by their `organization_id`.
2. Our `audit_events`, filtered by their `organization_id`.

We merge in time order, render to CSV or JSON, deliver. The compliance view in our admin tool does this for both internal and customer-facing flows.

**SOC 2 auditor asks for evidence of an incident.** Same mechanic. The auditor cares that the trail is complete and tamper-evident, not that it lives in one table.

Correlation across the boundary is via `workos_user_id` and `workos_organization_id` on every row of both systems, plus `workos_session_id` where the event has one.

### 7.5 Retention

We have not yet picked retention numbers. The constraints we know:

- SOC 2 Type II commonly expects ≥12 months of audit evidence.
- Some customer questionnaires ask for 7 years on billing-adjacent events.
- WorkOS Audit Logs default retention and export model — verify before launch. If WorkOS retains less than our policy commits to, we either (a) export to our own Blob on a schedule and never rely on WorkOS for the full window, or (b) align our policy down. Option (a) is the safer answer; export every 24 hours to `audit-logs-export/<org_id>/<date>.jsonl` in our Blob, retain there for 7 years.
- Our `audit_events` retention: provisional 7 years for billing-relevant events, 2 years for everything else. Revisit before SOC 2 Type I.

Worth noting: WorkOS Audit Logs export to Blob/S3 is a question to confirm during procurement, not assume. Open question, tracked at the bottom of this doc.

### 7.6 What if WorkOS goes down or loses data?

The honest answer: identity-event reconstruction is not possible from our side. Mitigations:

- The cross-cutting events double-recorded in `audit_events` give us a redundant trail of session establishment, membership change, and role change. That covers the events most likely to matter for incident response.
- Pure auth events (a single failed MFA attempt, a successful SSO redirect that didn't change anything) are not double-recorded; if WorkOS loses them we don't have them. This is acceptable given WorkOS's posture as a serious identity vendor and the rarity of full-loss scenarios.
- Daily audit-log export to our Blob (§7.5) closes the durability gap for everything WorkOS records.

---

## 8. Architecture

The full runtime, drawn against the rest of the stack from `azure-proposal.md`. Identity flow is highlighted; non-identity components are present but lightly sketched.

```
                         ┌─────────────────────────────┐
                         │     Customer's IdP          │
                         │  (Okta / Entra / Google /   │
                         │   AuthKit's own user store) │
                         └──────────────┬──────────────┘
                                        │  SAML / OIDC / OAuth
                                        │
                         ┌──────────────┴──────────────┐
                         │           WorkOS            │
                         │                             │
                         │  AuthKit (login UI, MFA,    │
                         │   sessions, JWTs)           │
                         │  Organizations + roles      │
                         │  SSO + Directory Sync       │
                         │  Audit Logs                 │
                         │  Admin Portal               │
                         └──┬────────────┬──────────┬──┘
              redirects     │            │ webhooks │  Audit Logs API
              + JWTs        │            │          │  (export / query)
                            ▼            ▼          ▼
   ┌──────────┐      ┌────────────────────────────────────────┐
   │ Browser  │ ───► │ Cloudflare (DNS, WAF, CDN)             │
   │ (React/  │      └──────────────────────┬─────────────────┘
   │   TS)    │                             │
   └──────────┘                             ▼
                       ┌──────────────────────────────────────────────┐
                       │  Azure Container Apps Environment            │
                       │                                              │
                       │  ┌──────────────────┐   ┌──────────────────┐ │
                       │  │   api (Go)       │   │  worker (Go +    │ │
                       │  │                  │   │   River)         │ │
                       │  │  • session       │   │                  │ │
                       │  │    middleware    │   │  • ParseDocxJob  │ │
                       │  │  • redirect      │   │  • ReviewRun     │ │
                       │  │    handlers      │   │  • LensRun       │ │
                       │  │  • webhook sink  │   │                  │ │
                       │  │  • domain APIs   │   │                  │ │
                       │  └────────┬─────────┘   └────────┬─────────┘ │
                       │           │                      │           │
                       └───────────┼──────────────────────┼───────────┘
                                   │                      │
                       ┌───────────┴──────┐    ┌──────────┴───────────┐
                       │ Postgres Flex    │    │  Azure Blob          │
                       │  • organizations │    │   (Hot, ZRS)         │
                       │  • departments   │    │  • contracts         │
                       │  • users         │    │  • derived markdown  │
                       │  • matters       │    │  • audit-log export  │
                       │  • audit_events  │    │     (24h scheduled)  │
                       │  • plans         │    └──────────────────────┘
                       │  • billing_      │
                       │     periods      │
                       │  • usage_events  │
                       │  • credit_ledger │
                       │  • reservations  │
                       │  • River jobs    │
                       │  • RLS by org_id │
                       └──────────────────┘

      api / worker  ◄─── webhooks ──── ┌─────────────────────────────┐
      (verify,                         │           Stripe            │
       dedupe,                         │                             │
       River dispatch)                 │  • Customer per Org         │
           │                           │  • Subscription             │
           │                           │      - licensed item        │
           └──── outbound API ───────► │      - metered item         │
              Customers.create         │  • Meter "arc_usage"        │
              CreditGrants.create      │  • Credit Grants            │
              Meters.createEvent       │     (quota + packs)         │
              Refunds.create           │  • Stripe Tax               │
                                       │  • Customer Portal          │
                                       │  • Embedded Checkout        │
                                       └─────────────────────────────┘

                       Foundry (Claude) and direct Anthropic
                         called from the worker — omitted here
                         for clarity; see Reviewer-v2 and
                         azure-proposal §2 for that path.
```

Three things this diagram makes concrete:

- **WorkOS sits between the customer's IdP and our app, never between our app and the customer's IdP.** We never speak SAML or OIDC ourselves. Every identity protocol terminates at WorkOS; we only ever consume WorkOS's normalized API.
- **The webhook fan-in is a one-way mirror.** WorkOS pushes events to a single API endpoint; the API writes to Postgres. We never pull state from WorkOS in the request path — that would put a third-party HTTP call on the user's request critical path. We pull on demand only for compliance views and for forced-resync admin operations.
- **Our Postgres holds the denormalized identity state and the entire domain.** Org boundary enforcement is RLS plus app checks, evaluated against Postgres rows that the webhook handler keeps fresh. WorkOS is not in the request path for authorization decisions beyond JWT validation.

---

## 9. Authentication flow

Three flows worth walking through end-to-end, since they're the ones that show up in design conversations.

### 9.1 Solo practitioner, email + password (or Google)

1. Browser loads `accordli.com/matters/42`.
2. React route guard sees no session cookie; redirects to `/auth/login` on our API.
3. API redirects to AuthKit's hosted login page, passing a state token and our redirect URI.
4. User enters email + password (or clicks "Continue with Google"). WorkOS verifies; if first-time, JIT-creates a User and Organization.
5. WorkOS redirects back to our API callback with an authorization code.
6. API exchanges the code for a session token, sets an HTTP-only secure cookie, redirects to the React app.
7. React route guard sees the cookie, fetches `/me`, gets `{user, organization, department, role}`, hydrates context, renders the page.

### 9.2 Firm member via the firm's SSO

1–3. Same as above, except the AuthKit login page detects the user's email domain matches a configured SSO connection.
4. AuthKit redirects the browser to the firm's IdP (Entra, Okta, etc.). The firm authenticates the user under their own MFA and conditional-access policies.
5. IdP redirects back to AuthKit with a SAML/OIDC assertion.
6. AuthKit JIT-creates the user (or updates them) inside the firm's Org, issues a session.
7–8. Same as steps 6–7 above. Our `/me` returns the firm's Org, the user's Department (default if not yet group-mapped), and role.

### 9.3 New user invited via SCIM

1. Firm IT admin creates a user in their IdP, assigns them to the Accordli app.
2. IdP pushes a SCIM `user.created` event to WorkOS.
3. WorkOS delivers it as a webhook to our API.
4. Our handler creates a `users` row, assigns to the firm's Org, default Department, default `member` role. Writes a `user.provisioned` row to `audit_events`.
5. (Optional, if configured.) WorkOS sends an invite email with a magic link to set up MFA / first sign-in.
6. User clicks the link. From here it's flow 9.2.

The deactivation flow is the mirror: SCIM `user.deactivated` → webhook → we revoke at the app layer (set `users.deactivated_at`) and force any active sessions to fail next request. Seat count is a local constraint enforced at invite time, not a Stripe-side meter, so deactivation does not generate a Stripe call. Orphaned rows (matters, reviews) stay assigned to the deactivated user; access is gated by `deactivated_at`.

---

## 10. Authorization model

Three layers, evaluated in order on every request. Each rejection is a 403, distinct per layer for debugging:

1. **Authentication.** WorkOS session JWT validates → API has `workos_user_id`. Failure: redirect to login.
2. **Tenant boundary.** API loads our local `users` row by `workos_user_id`, sets `app.current_org` GUC for this transaction. RLS clamps every subsequent query to that Org. Failure: 404 (don't leak existence across Orgs).
3. **Domain rule.** Per-route check. "Is this user an `org_admin` to access the billing page?" "Does this Matter's Department match the user's Department, or is the user an Org admin?" "Does the user's plan tier include the Risk Review type?"

WorkOS supplies layer 1. Layers 2 and 3 are ours.

A note on Postgres RLS: we adopt it from day one even at solo scale. The cost is roughly nothing (one policy per tenant-scoped table, one GUC set per request). The benefit is that any future SQL injection or query-builder bug cannot trivially leak one Org's data to another. Lawyers will ask about cross-tenant isolation; "RLS at the database layer" is a strictly better answer than "we are very careful in application code." `Reviewer-v2` already implies this; making it explicit here.

A note on FGA, again: skip in v1. The migration into FGA is real work but the alternative is real code we don't need yet. Don't spend the FGA money or take the FGA dependency until a customer needs it.

---

## 11. Billing integration in a Stripe-only world

MVP and Phase 1 are Stripe-only — no Orb, no Lago. The full billing spec is in `../product-specs/stripe-implementation-guide.md`. This section is just the WorkOS/Stripe seam.

The mapping:

```
WorkOS Organization ─┐
                     │  bridged in our DB:
                     ▼
              organizations.workos_organization_id
              organizations.stripe_customer_id
                     │
                     ▼
                Stripe Customer
                     │
                     ▼
                Stripe Subscription
                  (licensed item + metered item +
                   Credit Grants for quota and packs)
```

### 11.1 Why WorkOS's Stripe products still don't earn their place

WorkOS sells two Stripe-adjacent features. Now that we are Stripe-direct, this question is genuine, not rhetorical. Both still fail:

- **Stripe Entitlements integration.** Pushes Stripe subscription state into a JWT claim AuthKit issues, so app code can gate features off the session. Sounds attractive — it would mean a `past_due` Org's session token reflects the billing state. Doesn't fit because (a) our entitlement gate is **not** "do you have an active subscription"; it's "do you have available ARCs against `credit_ledger - active_reservations`," which Stripe Entitlements cannot represent. (b) The `billing_status` flag we *do* check is already mirrored locally by the Stripe webhook handler per `stripe-implementation-guide.md` §15. Adding a WorkOS-mediated path would duplicate the source of truth, not consolidate it.
- **Stripe Seat Sync.** Pushes WorkOS member count to a Stripe metered Price for per-seat billing. Doesn't fit because Accordli's plans are flat-fee with a fixed app-enforced seat cap (1 / 1 / 3 / 10 / per-deal). Seats are a feature gate, not a metered billing dimension. There is no Stripe Price counting seats to feed.

Both products solve real problems for B2B SaaS shapes that Accordli doesn't have. Skip them.

### 11.2 The seam, concretely

Org provisioning is **two-step on purpose**, with the steps owned by different code paths:

1. **WorkOS webhook handler** receives `organization.created`, inserts our local `organizations` row, leaves `stripe_customer_id` null. This handler does only identity work.
2. **Signup flow handler** (per `stripe-implementation-guide.md` §5) calls `stripe.Customers.create`, stores `stripe_customer_id`, then opens Embedded Checkout for the chosen plan. This handler does only billing work.

These run in different request contexts because Orgs can be created without a paying user immediately attached (e.g., SCIM-provisioned firms where the first user lands later). Coupling Stripe Customer creation to the WorkOS webhook would mean either creating Stripe Customers we never use, or blocking webhook processing on a third-party API call — both bad.

The signup handler is the single point where the local Org row gets its `stripe_customer_id` written. After that, Stripe webhooks (`invoice.created`, `invoice.paid`, `charge.refunded`, etc.) drive all subsequent billing-state changes. WorkOS is not in this loop.

### 11.3 Membership changes and seats

User joins or leaves an Org → WorkOS membership webhook → local `users` row inserted/marked deactivated → app enforces the plan's seat cap before letting an admin invite past it. There is no per-seat Stripe meter to update because Accordli doesn't price by seat in MVP/Phase 1. Adding per-seat pricing later would be a `stripe-implementation-guide.md` change, not a WorkOS change.

### 11.4 The "manage billing" deep-link

When an Org admin clicks "manage billing" in our app, we deep-link them into Stripe's hosted Customer Portal using their `stripe_customer_id` per `stripe-implementation-guide.md` §12. The session that authorizes this deep-link is the WorkOS session (we wouldn't generate the link without a valid session and `org_admin` role). Stripe Customer Portal then runs its own short-lived auth, independent of WorkOS.

This is the cleanest seam shape: WorkOS authenticates the human; our app authorizes the action; Stripe owns what happens after the redirect.

---

## 12. Implementation phasing

### Phase 1 — Pre-launch (months 0–3)

Goal: the auth substrate is live before the first paying customer.

- WorkOS project, AuthKit configured, dev + staging + prod environments.
- Mirror schema (`organizations`, `departments`, `users`) plus migrations.
- Sign-in / sign-up redirect handlers; React route guard; `/me` endpoint.
- Session middleware in Go; RLS policies on every tenant-scoped table.
- WorkOS webhook handler covering `user.*`, `organization.*`, `organization_membership.*` (idempotent on event id).
- `audit_events` table; cross-cutting double-recording for `session.established`, `org_member.added`, `org_member.removed`, `role.changed`.
- Signup handler that creates the Stripe Customer and opens Embedded Checkout per `stripe-implementation-guide.md` §5 — runs after WorkOS authentication, separately from the WorkOS webhook fan-in.
- Daily audit-log export job (WorkOS Audit Logs API → our Blob, partitioned by Org).
- One end-to-end smoke test: signup → WorkOS Org provisioned → Stripe Customer created → user lands in app → ARC reservation works → audit trail joinable.

### Phase 2 — First firm customer (target: month 4–8)

Goal: a 5–50 lawyer firm signs up, requests SSO, walks through self-serve setup with their IT admin.

- Provision first SSO connection.
- Wire Admin Portal hand-off (signed link delivered to the firm's IT admin).
- Map the firm's SSO `domain → organization_id` in our DB so email-domain detection routes correctly at AuthKit.
- Test: SSO sign-in for an IT admin, JIT user provisioning, role assignment.
- Update pricing page and sales material to reflect SSO as an Enterprise / firm-tier feature with the $125/mo line item.

### Phase 3 — First enterprise customer (target: month 9–14)

Goal: AmLaw 200-ish firm or in-house team requires SCIM + audit-log export + custom security review.

- Add Directory Sync (SCIM) to the firm's existing SSO connection.
- Webhook handlers for `user.provisioned`, `user.updated`, `user.deactivated`, `group.*`.
- Customer-facing audit log export endpoint; admin-tool compliance view that joins WorkOS Audit Logs + our `audit_events`.
- Possibly: IdP-group → Department mapping per firm.
- Vanta or Drata wired into both WorkOS and our app for SOC 2 evidence collection.

### Phase 4 — Earned, not predicted

These adoptions wait on a real signal:

- **WorkOS FGA.** Wait for the first customer requirement that built-in roles plus Department scoping cannot satisfy. Likely a partner/associate matter-visibility ask.
- **WorkOS Radar.** Wait for measurable signup abuse.
- **Custom branded login (full Admin Portal branding).** Wait for a firm to ask for it.
- **HIPAA BAA with WorkOS.** Wait for the first health-system customer; verify WorkOS BAA is included on the tier we're on or upgrade as needed.

---

## 13. Open questions and risks

Verifications and decisions still owed before launch:

1. **WorkOS Audit Logs API retention default and export model.** If retention is under 12 months, daily-export-to-Blob (§7.5) is mandatory rather than belt-and-braces. Confirm during procurement.
2. **WorkOS Audit Logs filterable by Organization.** Required for customer-facing audit-log exports. Strong assumption that this works; verify the API shape.
3. **IP address propagation.** Do WorkOS session events expose the originating IP cleanly? Needed for our `audit_events` IP column on cross-cutting double-records.
4. **WorkOS HIPAA / BAA posture.** Verify if/when a health-system contract appears.
5. **WorkOS data residency.** Where do their hosted services live? Lawyer customers occasionally insist on US-only or EU-only data residency for identity data.
6. **Vendor risk and lock-in.** WorkOS owns user records and SSO/SCIM configurations. Migration path: identities re-bind via email + password reset; SSO connections re-config per Org; SCIM re-provision; Audit Logs export and never re-import. Painful but not catastrophic. Worth a one-page "what if we had to leave WorkOS" doc before signing a multi-year contract.
7. **AuthKit branding ceiling.** How far can the hosted login UI be skinned without paying for full Admin Portal customization? Affects whether the solo-practitioner login feels Accordli-native or WorkOS-native.

Cross-references to other open work:

- `todo.md` item 4 (operational dashboards) — WorkOS console covers identity ops; we still build our own admin console for matters/billing.
- `todo.md` item 12 (solo practitioner data model) — recommendation: real Org and Department rows with `is_default = true`, hidden in UX. Solos should not be a special case in the access-control code.
- `todo.md` item 10 (Org vs Department billing tensions) — orthogonal to WorkOS.
- `todo.md` item 9 (signup flow) — picks up where §9 leaves off; WorkOS handles auth, our flow handles plan choice and Stripe Customer provisioning per `stripe-implementation-guide.md` §5.

---

## 14. The decision in one paragraph

WorkOS is the right tool for our job. Adopt AuthKit, Organizations, and Audit Logs from day one — all free. Add SSO when the first firm asks ($125/Org/mo, passed through). Add SCIM with the first enterprise customer. Defer FGA, Radar, and both WorkOS-Stripe integrations (Entitlements doesn't model our ARC-balance gate; Seat Sync doesn't fit our flat-fee plans with fixed seat caps). Mirror identity into our Postgres for performance and RLS, but never duplicate authentication concerns. Keep `audit_events` strictly for domain events, double-record only the small set of cross-cutting events that change authorization or billing state, and join WorkOS's log with ours at compliance-view time. The Stripe seam is intentionally simple: WorkOS authenticates, our signup handler creates the Stripe Customer, Stripe webhooks drive billing state from there. The architecture survives going from solo-practitioner-only to a regulated enterprise customer with one new WorkOS connection per firm and zero re-platforming on our side — which is the whole point of paying for it.

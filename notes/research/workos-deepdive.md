# WorkOS Deep Dive — Research Log

Investigation into whether WorkOS is the right identity / authorization
substrate for Accordli analyze. The brief: solo → small team → large team →
enterprise customers, security-forward at launch, SOC 2 Type I ~month 9, lawyer
audience, two-engineer team. Locked-ish in current notes; nothing committed.

This file is the working log. Findings, open questions, and the eventual
recommendation all live here. Updated as the investigation proceeds.

---

## Phase 1 — Concerns and Questions Driving the Investigation

These are the questions whose answers will determine whether WorkOS is a
"yes, default to this" or a "yes, but with these caveats" or "no, here's why
not." Grouped by axis.

### A. Fit with our identity / data model

1. **Organization / Department / User mapping.** WorkOS has Organization and
   User as first-class concepts. Accordli has Org → Department → User. Does
   WorkOS represent Department natively? If not, do we model Department in our
   own DB and use WorkOS only for Org+User, or use WorkOS Groups / RBAC roles
   to express Department, or use FGA?
2. **Solo practitioner ergonomics.** Solo signups need a dead-simple flow
   (email + password, or Google). Does AuthKit's hosted UX gracefully hide
   "select organization" / "join workspace" steps for a single-user account?
   Can the same Org row work invisibly for solos and visibly for firms?
3. **Cross-Org user identity.** Lawyers move firms; one human may end up with
   multiple Org memberships over time. Does WorkOS support a single User
   across multiple Organizations, or is User scoped per Org? (This affects
   data export and audit trails.)
   
   Tom: We don't care about this right now.
   
4. **Mid-flight tenancy promotion.** A solo on a Pro plan invites a partner;
   the account becomes a small team. Does the model support that without
   forcing a re-signup, or do we need to design a "promote Org" path?
   
   Tom: Yes we do need to design a "promote org" path.

### B. Surface area: what we get vs. what we still build

5. **AuthKit hosted UI vs. embedded SDK.** What's the default? Hosted UI
   (workos-managed login pages) vs. headless / embedded React components.
   Implication for our design system, branding, and lawyer-grade trust signals.
   
   Tom: We prefer React components for most things where it makes sense.
   
6. **MFA, password reset, magic link, social, passkeys.** Which are included
   in AuthKit out of the box, which are paid add-ons, which require config
   work from us?
7. **Session management.** Token format, refresh, revocation, "log out
   everywhere," device list. What's API-accessible vs. only in their UI?
8. **Invite flow.** Org-admin invites a teammate. Does WorkOS handle the
   email, the accept-link, the join-org-on-signup join? Or do we send the
   email and call WorkOS APIs to mint memberships?
9. **Admin Portal.** Does this give the customer's IT admin a useful self-
   serve experience for SSO and SCIM, or is it a pretty wrapper that we
   still have to integrate against?
   
   Tom: Let's understand what a sample flow would be to integrate an SSO customer.
   
10. **RBAC vs. FGA.** When does the line cross from "roles are enough" (Owner,
    Admin, Member) to "we need fine-grained authorization"? Department-scoped
    Matter visibility looks like an FGA-shaped problem; or does it stay simple
    with org_id + dept_id checks in our own code?

### C. Audit logging boundary

11. **What WorkOS Audit Logs capture by default.** Identity events presumably:
    sign-in, MFA enrollment, SSO config change, SCIM provisioning. Do they
    capture _our_ application events (Matter created, Review run, finding
    exported), or are those strictly our responsibility?
12. **Where the boundary should live.** Two-table model (`audit_events` in our
    DB + WorkOS Audit Logs) vs. unified (everything funneled to WorkOS) vs.
    inverted (everything in our DB, WorkOS data mirrored in). Each has
    consequences for SOC 2 evidence, for customer-facing audit-log UI, and
    for log streaming to enterprise SIEMs.
13. **SIEM streaming.** Enterprise customers will want logs streamed to their
    Splunk / Datadog / Sentinel. Does WorkOS's "$125/mo per SIEM connection"
    cover only WorkOS-recorded events, or can we send our own events through
    that pipeline too?
14. **Retention and pricing of audit log volume.** $99/mo per 1M events at
    today's pricing. What's our event rate likely to be? At what customer
    scale does this become a meaningful line item?

### D. SOC 2 and compliance posture

15. **What WorkOS can show us.** SOC 2 Type II, ISO 27001, GDPR, HIPAA BAA
    availability. Do we get their reports under NDA without a sales call?
16. **Subprocessor implications.** Adding WorkOS adds a subprocessor on the
    list lawyers will read. Is the brand recognized enough that this is a
    plus, or does it introduce friction with regulated/in-house customers
    who prefer Okta / Entra?
17. **Data residency.** Where does WorkOS host identity data? US only? EU
    region available? Relevant for any future EU customer.
18. **DPA, BAA, MSA.** Can we sign these without a sales motion at our scale?
    What's the smallest plan that includes them?

### E. Pricing and commercial fit

19. **AuthKit's "first 1M MAUs free" claim.** Real, durable, or marketing?
    What turns from free into paid first — extra connections, audit log
    events, Admin Portal, custom domain?
20. **Per-connection SSO/SCIM cost.** $125 per connection per month for the
    first 15. At what plan tier do we charge customers for SSO (the classic
    "SSO tax")? Does the unit economics of Small Team / Large Team plans
    cover this if 30% of those orgs ask for SSO?
21. **Custom domain $99/mo.** Do we need this on day one for our lawyer-grade
    brand, or can we live on `auth.accordli.com` via DNS without paying?
22. **Hidden line items.** Vault, FGA, Radar, MFA — what's metered, what's
    flat, what's gated to enterprise tiers.
23. **Annual vs monthly, minimums, startup program.** Does WorkOS have a
    YC / startup deal that materially changes the math?

### F. Risk / lock-in / vendor health

24. **Migration off.** If we wanted to leave in year 3, can we export users,
    org memberships, hashed passwords, MFA factors, audit logs? Which of
    those _can't_ be migrated and would force a forced password reset?
25. **Long-term vendor health.** WorkOS is well-funded and growing as of
    early 2026, but identity is a long-tail commitment. What's the Plan B
    if they get acquired and the product changes character (cf. Auth0/Okta)?
26. **Outage / failure mode.** WorkOS is in the hot path of every login.
    Their uptime history, incident communication, and our fallback if
    AuthKit is down for hours. (Probably no fallback; people just can't log
    in. Confirm if that's acceptable.)
27. **Practical limitations users have hit.** What do experienced WorkOS
    customers complain about on HN / Reddit / blogs? Particularly around
    hosted UI flexibility, multi-org users, and the AuthKit → enterprise SSO
    transition.

### G. Build vs. buy decision frame

28. **What would we build if we self-hosted (Ory / Zitadel)?** Cost in
    engineer-weeks for: AuthKit-equivalent UX, Org/Member CRUD, SSO ingest,
    SCIM consumer, audit log UI, Admin Portal, MFA, magic link, password
    reset. Compare to WorkOS price at expected scale.
29. **What does Clerk give us that WorkOS doesn't, and vice versa?** Clerk's
    React UX is regularly cited as best-in-class; their B2B / enterprise
    story is weaker. For lawyers buying us, which matters more?
30. **Reversibility window.** Given that auth is sticky, when is the latest
    we could switch off WorkOS without re-signing every user? (Partly
    determined by #24.)

### H. Integration with Stripe

31. **What does a signup flow look like** that includes choosing a plan and paying for it?
32. **Cancelation** When a user cancels an account, how is that handled?

### I. Integration with our Accordli app

33. How does login work?
34. How does signup work?
35. How does signout work?
36. How does cancelation/termination work?

---

## Phase 2 — WorkOS Product Surface (Findings)

### Major product areas (snapshot)

| Product | One-line | Notes |
|---|---|---|
| AuthKit | Hosted authentication: email/password, social, magic auth, MFA, sessions | The default frontend on top of User Management |
| User Management | Users, organizations, memberships, invites, password & email primitives | The data layer AuthKit sits on |
| SSO | SAML / OIDC connectors per customer org | Per-connection pricing |
| Directory Sync | SCIM (and HRIS) consumer for user lifecycle | Per-connection pricing |
| Admin Portal | Self-serve UI for customer IT admins to configure their SSO/SCIM | Branded, link-in flow |
| Audit Logs | Ingest + retain + stream identity and app events | $/event + $/SIEM connection |
| RBAC | Roles + permissions, org-scoped | |
| FGA | Fine-grained auth (Zanzibar-style) | Separate product |
| Vault | Encrypted KV / secret storage | |
| Radar | Bot / fraud / abuse signals on login | |
| Pipes | Customer connects 3rd-party accounts | |
| Feature Flags | Flag delivery | |
| Widgets | Pre-built UI for common enterprise flows | |
| Domain Verification | Customer claims a domain | Used to gate SSO defaults |

### Pricing snapshot (as of May 2026 — confirm before quoting)

| Item | Price |
|---|---|
| AuthKit MAU | First 1M free; $2,500/mo per additional 1M |
| SSO (per connection per month) | $125 → $100 → $80 → $65 → $50 sliding by volume |
| Directory Sync (per connection per month) | Same scale as SSO |
| Audit Logs — SIEM streaming | $125/mo per connection |
| Audit Logs — event retention | $99/mo per 1M events |
| Radar | First 1k checks free; $100/mo per 50k |
| Custom domain | $99/mo |
| Admin Portal / Vault / FGA / MFA | Pricing not on public page; pull from sales or current docs |

> Marketing claim: "Free to start, you only pay for what you use." No
> minimum confirmed, but the public pricing page omits Vault, FGA, Admin
> Portal, and MFA pricing. Validate before we lean on the math.

### Data model (verified)

- **User.** Unique by email address. One User can have multiple auth methods
  attached (email/password, Google OAuth, SSO, passkey, MFA). WorkOS does
  identity linking automatically on the email key.
- **Organization.** A grouping of users that "provides structure to manage
  and enforce authentication methods and resource access." Has standard
  fields (`id`, `name`, `domains[]`), supports `external_id` (≤64 chars,
  unique within the WorkOS environment) and **custom metadata** (≤10
  string key-value pairs; key ≤40 chars; backend-only).
- **OrganizationMembership.** The join row. Fields: `id`, `user_id`,
  `organization_id`, `role_slug` (or `role_slugs[]` if multi-role enabled),
  `status` ∈ {`pending`, `active`, `inactive`}. Standard CRUD + dedicated
  `deactivate` / `reactivate` endpoints.
- **A user can belong to multiple Organizations.** Confirmed: "A self-serve
  productivity app, like Figma, where each user can be in any number of
  organizations." There is an Organization Switcher widget for the user
  to flip context. _(Tom marked this as not-currently-relevant; noted
  for future.)_
- **No native sub-Organization / Department concept.** Directory Group is a
  thing but it's a SCIM-import construct (mirroring the customer's IdP
  groups), not a freestanding hierarchy primitive. **For Department we
  must model in our own DB** and link via `external_id` or metadata on
  the Membership / a sibling table.

### Sessions and tokens (verified)

- **Access token = JWT.** Claims include `sub` (WorkOS user id), `sid`
  (session id), `iss` (`https://api.workos.com/`), `org_id` (selected org
  at sign-in), `role`, `permissions` (array — populated from RBAC), `exp`,
  `iat`. JWT templates can extend claims to include metadata values from
  org/user (so e.g. `accordli_dept_id` could ride in the token).
- **Refresh token rotates on use.** "Refresh tokens may be rotated after
  use, so be sure to replace the old refresh token with the newly returned
  one." Standard OAuth2 refresh-token-rotation pattern.
- **Storage on the frontend.** Access token in a secure cookie; refresh
  token either in a secure cookie or held server-side. The Next.js SDK
  defaults to the cookie pattern.
- **Validation in Go.** Pull JWKS from
  `https://api.workos.com/sso/jwks/<clientId>`, verify with a standard
  library (`go-jose`, `lestrrat-go/jwx`). Standard pattern; no WorkOS-
  specific quirks.
- **Signout.** Hit the `/logout` endpoint with the `sid`. This kills the
  session at WorkOS. There is **no documented "log out everywhere" / list
  active sessions** API surfaced — sessions are revoked one at a time.
  Open question: do we expose device/session management to users? If yes,
  we likely need to track sessions in our own DB on top, or rely on
  `session.revoked` webhooks.

### RBAC and FGA (verified)

- **RBAC.** Roles defined at two levels: environment-scoped (defaults that
  apply to every Org — e.g. `member`, `admin`) and Organization-scoped
  (custom roles, slug auto-prefixed `org_`, configurable per customer).
  Roles attach to Memberships. Permissions ride in the JWT under
  `permissions[]`. Enforce in Go middleware on each API call by reading
  `permissions` from the validated JWT.
- **FGA.** Hierarchical resource model: Subjects (users/groups/agents) +
  Resources (with parent links — first-class hierarchy) + Privileges
  (roles/permissions). p95 access checks <50ms, strong consistency.
  Marketed as Zanzibar-style in spirit (relationship-based) but uses a
  hierarchy + privileges model rather than pure tuple semantics. **FGA
  pricing is not on the public page.** Likely sales-call territory.

### Audit Logs (verified)

- **WorkOS-emitted events.** Identity events (sign-in, MFA enrollment, SSO
  connection lifecycle, SCIM provisioning) are emitted automatically.
- **App-emitted custom events.** Yes, supported. We register event
  schemas in the dashboard (`action` name, allowed `target` types, optional
  metadata schema), then call `auditLogs.createEvent(orgId, {...})` from
  our Go SDK. Schema fields: `action`, `actor` (type+id), `targets[]`
  (type+id), `occurred_at`, optional `context` (location, user agent),
  optional `idempotency_key`.
- **Same stream.** App-emitted custom events live alongside WorkOS native
  events in the same audit log store, scoped per Organization. CSV export
  through Dashboard or API; **events from the past 3 months are
  exportable** (note: this is the export window, not necessarily total
  retention).
- **Log Streams (SIEM).** Six destinations: Datadog, Splunk, AWS S3, GCS,
  Microsoft Sentinel, Generic HTTPS. Customer's IT admin self-configures
  via Admin Portal. Both auto-emitted and custom app-emitted events flow
  through the same stream — confirmed for the WorkOS pipeline (since they
  share the same store).
- **Customer-facing UI.** Customers see their org's audit log in the WorkOS
  Dashboard (when they have an admin login there). For an _embedded_
  audit log inside our app's UI, we'd build it ourselves on top of the
  Audit Log Events API.

### Admin Portal (verified)

- A WorkOS-hosted, branded self-serve interface for the customer's IT
  admin, accessed via a setup link we generate from our backend.
- Covers: SSO connection setup (with IdP-specific instructions for Okta,
  Entra ID, OneLogin, Google, Auth0, …), Directory Sync (SCIM)
  configuration, Domain Verification, and Audit Log Streaming destination
  config.
- Each setup link is per-organization. Email it to the contact ourselves,
  or have WorkOS send it. WorkOS displays connection status (pending →
  active) on the Dashboard and notifies on cert expiry within 90 days.

### Stripe add-on (verified)

- Two pieces: **Stripe Entitlements** (subscription-based features ride in
  the JWT as entitlements claims) and **Stripe Seat Sync** (push WorkOS
  member counts to Stripe metered billing). Both surface-level integrations
  on top of an Org ↔ Stripe Customer link we maintain.
- The link is a `stripeCustomerId` field on the WorkOS Organization
  (settable via `updateOrganization`). We are responsible for creating the
  Stripe Customer and writing the id back. WorkOS does not subscribe to
  Stripe webhooks for us; subscription state is reflected via
  re-authentication / token refresh ("Entitlements will show up in the
  access token the next time the user logs in or the session is
  refreshed").
- **Implication:** WorkOS gives us a clean place to attach the Stripe link,
  but the cross-system orchestration (subscription state → entitlement
  → access policy) is still ours. Stripe webhook handling, dunning,
  cancel-at-period-end logic all live in our Go code.

### Migration (verified)

- **Into WorkOS:** Bulk import of users with hashed passwords supported.
  Hash formats accepted: scrypt, bcrypt, argon2, pbkdf2.
- **Out of WorkOS:** No public migration-out guide. We can pull users,
  orgs, memberships, audit log events via API, but **password hashes are
  not exportable** — a forced password reset is the realistic exit path.
  This is the canonical "auth lock-in" gotcha and is industry-standard
  (it's also true of Auth0, Clerk, Supabase Auth).

---

## Phase 3 — Mapping to Accordli's Data Model

### The recommended model

```
Accordli DB                         WorkOS
───────────                         ──────
organizations                ←→     Organization
  id (uuid, ours)                     id (org_xxx)
  workos_org_id (FK)                  external_id  ← we set to our uuid
  name                                name
  metadata (jsonb)                    metadata (10kv) ← cache critical bits
  stripe_customer_id          ←→     metadata.stripe_customer_id
                                          (or top-level stripeCustomerId)

users                        ←→     User
  id (uuid, ours)                     id (user_xxx)
  workos_user_id (FK)                 external_id  ← our uuid
  email                               email
                                      metadata.accordli_dept_id
                                          ← mirrored from our DB,
                                            promoted to JWT claim

memberships                  ←→     OrganizationMembership
  id                                  id (om_xxx)
  workos_membership_id (FK)
  user_id (ours)
  organization_id (ours)
  department_id (FK)  ← OURS         role_slug
                                      status

JWT (issued by WorkOS):
  sub, sid, iss, org_id, role, permissions[], exp, iat,
  accordli_dept_id   ← custom claim from JWT template,
                       sourced from user.metadata.accordli_dept_id

departments                  ←→     [no native counterpart — ours alone]
  id
  organization_id
  name

matters
  id
  department_id  ← canonical owner
  organization_id (denormalized for RLS)
  ...
```

**Department lives in our DB only.** WorkOS has no sub-org primitive that
fits cleanly. Three options considered:

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Department as a row in our DB; FK on memberships and matters | Simple, matches our existing spec, RLS in Postgres covers tenancy | We own the model entirely | ✅ Pick this |
| Use Directory Group from SCIM | Free for SCIM customers; mirrors customer IdP | Only populated when customer uses SCIM; useless for self-serve solos | ❌ |
| Use FGA with Department-scoped resources | Future-proof for arbitrary visibility rules | Adds a vendor product, costs money, latency on every check, premature | ❌ for v1; revisit when (10b) gets real |

For RBAC, use WorkOS env-level roles (`owner`, `admin`, `member`) and let
each Org override via custom roles when the customer asks. Departmental
access comes from our own `dept_id` checks in API middleware, not from
WorkOS RBAC.

**Department id rides in the JWT — decision.** The dept_id join is on
the hot path of every authenticated request (every Matter / Review API
call needs it for tenant + dept scoping). Going to Postgres on every
request is wasteful when the value is stable for the life of a session.
We promote it into the access token via a **JWT template** custom claim
(`accordli_dept_id`). API middleware reads it directly off the validated
JWT — no DB hit on the read path.

Mechanism:

1. We mirror our `department_id` to **WorkOS user metadata** under key
   `accordli_dept_id` (single string value). Set it whenever the user's
   department changes (initial signup, dept move, promote-org).
2. Configure a JWT template in the WorkOS dashboard that maps
   `user.metadata.accordli_dept_id` → claim `accordli_dept_id` on the
   access token.
3. Go middleware, after JWKS-validating the JWT, attaches
   `dept_id = claims["accordli_dept_id"]` to the request context. No
   DB hit.
4. On dept change, force a token refresh so the new claim takes effect
   without waiting for token expiry. (Same pattern Stripe entitlements
   uses — re-auth picks up the new claim.)

**Why user.metadata, not membership.metadata.** WorkOS supports
metadata on User and Organization but does not document membership
metadata. Our model is single-Org-per-User (Tom's note on Q3 confirms
multi-Org isn't a concern in the foreseeable future). One user → one
dept → user.metadata is the right home.

**If multi-Org becomes real later.** user.metadata can't represent
"different dept per Org." Two options when that day comes: (a) use
organization.metadata keyed by user (clunky, doesn't scale to many
users), or (b) drop the JWT template and look up dept from our own
membership row on each request (cache it; the Postgres read is cheap).
Don't pre-build for it now.

**Audit trail invariant.** Because the JWT carries `accordli_dept_id`,
every audited request can attribute "who in which dept" without an
extra read. Useful for the audit-event helper and for SOC 2 evidence.

### Solo practitioner ergonomics

Behaviorally invisible Org+Department for solos:
1. AuthKit signup (email/password or Google).
2. Server-side: create one Organization (auto-name "<email>'s workspace"),
   create one Department ("Default"), create the OrganizationMembership
   with role `owner`, attach Stripe customer, kick off subscription flow.
3. UI never shows org-pickers, dept-pickers, or invite flows for solos.
   Gate that on `org.metadata.is_solo === "true"` (or a column in our DB).

When the solo upgrades to a team plan _and_ adds their first additional
user, flip the flag and reveal the team UX. (See "Promote-org path"
below.)

### Mid-flight tenancy promotion (Tom: "yes, design this")

The simplest defensible path:

1. **State.** Solo's Org row has `tier = "solo"` (in our DB). Both Org and
   Dept exist; UI just hides them.
2. **Trigger.** Solo clicks "Invite a teammate" or "Upgrade to Small Team"
   on the account page.
3. **Plan change.** Stripe subscription is updated (Pro → Small Team) via
   Stripe API; we own this.
4. **Flag flip.** Set `tier = "team"` and `is_solo = false`. Surface
   the team UX (org switcher, member list, invite flow, dept admin).
5. **Org name.** Prompt the user to set a real Org name and Department
   name(s). Default names come along for free.
6. **Invite.** Use WorkOS's invitation API to invite the new teammate;
   WorkOS sends the email and handles the accept link.

Critically, **the Organization id and User id never change**. All Matters,
Reviews, Findings, audit events stay attached. No data migration. This is
the load-bearing reason to materialize the (default) Org and Dept on day
one for solos rather than creating them lazily on first invite.

---

## Phase 4 — Pricing Projections

### Public price points (May 2026, confirm before quoting)

| Item | Price | Trigger |
|---|---|---|
| AuthKit MAU 0–1M | Free | All MAUs in the first 1M |
| SSO connection | $125/mo (sliding to $50 at 200+) | Per customer that turns on SSO |
| Directory Sync connection | Same scale as SSO | Per customer with SCIM |
| Audit Log SIEM stream | $125/mo per connection | Per customer streaming to SIEM |
| Audit Log retention | $99/mo per 1M events | Volume-based |
| Custom domain | $99/mo | One-time-per-environment |
| Radar (bot/fraud) | First 1k checks free; $100/mo per 50k | Optional |
| Vault, FGA, Admin Portal, MFA | Not on public page | Sales-call required |

### Back-of-envelope projections

Anchoring assumptions: solo plan $200, team plans $600 / $2000, enterprise
custom. From `accordli_platform_overview.md`. Adoption shape: assume 70%
solo-only, 25% small team, 4% large team, 1% enterprise at the customer-
count level. SSO adoption among teams: ~30% of small teams ask, ~70% of
large teams ask, 100% of enterprise.

**Year 1, 100 paying orgs:**
- ~70 solo, ~25 small team, ~4 large team, ~1 enterprise.
- AuthKit MAU: ~250 MAU. Free.
- SSO connections: ~25 × 30% + 4 × 70% + 1 = ~11 connections × $125 = **$1,375/mo**.
- Directory Sync: assume same set of customers want SCIM = **~$1,375/mo**.
- Audit Log retention: at one Review per ARC and ~5 events per Review,
  100 orgs × ~30 ARC/mo × 5 events ≈ 15k/mo. Far below the $99/1M floor,
  effectively $99/mo.
- Custom domain: $99/mo.
- **Total: ~$2,950/mo**, or about $35k/yr at the 100-customer mark.

**Year 2, 500 paying orgs (same shape):**
- AuthKit MAU still in free tier (~1.2k MAU).
- SSO connections: ~52 × $100 (volume tier) = **$5,200/mo**.
- Directory Sync: **~$5,200/mo**.
- Audit Log retention: 500 × 30 × 5 = 75k events/mo → still $99/mo.
- Custom domain $99.
- **Total: ~$10,600/mo, ~$127k/yr.**

**Solo-only scenario, 100 solo practitioners (no teams, no enterprise):**

| Item | Cost | Why |
|---|---|---|
| AuthKit MAU | $0 | ~80 MAU, deep in the 1M free tier |
| SSO connections | $0 | Solos don't have SSO |
| Directory Sync | $0 | Solos don't have SCIM |
| Audit Log retention | $0–$99 | If we rely on our DB as canonical (Phase 6 dual-write), we can skip WorkOS retention entirely. Add $99/mo only if we want the WorkOS-dashboard audit view for internal support |
| Audit Log SIEM streaming | $0 | Nobody asks for SIEM at solo scale |
| Custom domain | $99 | Want it for brand from day one |
| Radar / Vault / FGA / MFA | $0 | Not needed |
| **Total** | **~$99–$198/mo** | |

Volume-anchor on event counts: 100 solos × ~12 ARCs/mo × ~10 events
per Review ≈ 12k application events; plus ~2k identity events from
sign-ins / MFA enrollments. Total ~15k events/mo — comfortably below
the $99/1M-events floor, which is why Audit Log retention is the only
discretionary line item in the table.

Revenue context: 100 solos at a 70/30 Pro/Gold split = ~$26K MRR. WorkOS
cost is ~0.4–0.8% of revenue. Functionally free at this scale; the
substrate doesn't pay back its own price in dollars until the first
SSO-paying customer lands. Pays back in engineering time saved every
day before that.

**Two implications worth noting:**

1. **The SSO tax is the entire pricing story.** All other line items
   (MAU, audit log, Radar, custom domain) sum to ~$200/mo at any
   plausible scale up to ~10K MAU. SSO + SCIM are what scale costs
   meaningfully with customer count. A solo-only book of business has
   essentially zero WorkOS cost.
2. **Don't pay for Audit Log retention by default.** Our dual-write
   pattern (Phase 6) makes our Postgres canonical. Skip the $99/mo
   retention line until we have a specific reason to keep events in
   WorkOS for >30 days — e.g. an enterprise customer who requires a
   WorkOS-dashboard audit view, or SIEM streaming that requires
   on-WorkOS retention to backfill into.

**Solo + small-team scenario, 100 solos + 20 small teams (no large
team, no enterprise):**

Counts:
- 120 Organizations (100 solo + 20 team)
- 160 Users (100 solo + 60 team users at 3 seats × 20 teams)
- ~130 MAU
- 0 SSO connections (per the Phase-4 policy: don't offer SSO on
  Small Team — $125/mo on $600/mo revenue is 21% margin burn)
- 0 SCIM connections (same reason)

| Item | Cost | Why |
|---|---|---|
| AuthKit MAU | $0 | 130 MAU, deep in the 1M free tier |
| SSO connections | $0 | Gated behind Large Team — none here |
| Directory Sync | $0 | Same |
| Audit Log retention | $0 | Postgres canonical (Phase 6 dual-write); skip the $99 retention line |
| Audit Log SIEM streaming | $0 | No enterprise customer asking |
| Custom domain | $99 | Brand-day-one |
| Radar / Vault / FGA / MFA | $0 | Not needed |
| **Total** | **~$99 / mo** | |

Volume-anchor on event counts: 100 × 12 + 20 × 40 = 1,200 + 800 = 2,000
ARCs/mo × ~10 events per Review ≈ 20k application events. Plus ~3k
identity events from 130 MAU. Total ~23k events/mo — still trivially
within Postgres at any retention window we'd want.

Revenue context:
- Solos at 70/30 Pro/Gold split = ~$26K MRR
- Small teams at $600/mo = $12K MRR
- **Total ~$38K MRR**
- WorkOS at $99/mo is **~0.26% of revenue.**

**Comparison to mixed-adoption shape.** The earlier "100 paying orgs,
mixed adoption" projection landed at ~$2,950/mo because it assumed
~11 SSO connections live. The same customer count with SSO gated
behind Large Team (i.e. _our policy_ rather than the unconstrained
adoption shape) is the $99/mo number above. **The cost discipline of
"don't sell SSO on Small Team" is worth ~$2,800/mo at 100 customers.**
That's the load-bearing finding for plan design.

**What changes if we relaxed the policy.** Even charitably, if we did
sell SSO on Small Team and 30% of them turned it on:

- 6 SSO × $125 = $750/mo (+$750)
- 6 SCIM × $125 = $750/mo (assume same set wants SCIM) (+$750)
- New total: ~$1,599/mo (~4% of revenue, still healthy in aggregate)
- But _per-SSO-customer_ margin: $125 SSO + $125 SCIM = $250/mo on a
  $600/mo plan = 42% of one customer's revenue gone to WorkOS.

Aggregate looks fine; the per-customer math is what kills it. A
single SSO+SCIM small-team customer is now contributing $350/mo gross
margin (before LLM costs, infra, support). Once you subtract LLM cost
on their 40 ARCs (~$66/mo at the mid case from §4.2 of the Azure
proposal) and a slice of fixed infra and support, that customer is
break-even or slightly negative. Hence the policy.

### Margin check on the SSO tax

Small Team plan is $600/mo. One SSO connection is $125/mo. That's 21% of
the plan's revenue if a small team turns on SSO. **This is the headline
finding on pricing:** if we offer SSO on Small Team, we burn 20% of
revenue per SSO-enabled small team customer. Industry standard solution
is to gate SSO behind Large Team plan ($2,000/mo, where $125 is 6%) or
make it Enterprise-only. _Decision: do not offer SSO on Small Team._

Same math applies to Directory Sync. Bundle SSO + SCIM together at the
Large Team tier; Enterprise gets it always.

### Watch-list line items

- **Custom domain $99/mo** — visible to the customer (login page lives at
  `auth.accordli.com`). Worth it day one for brand. Cheap.
- **Radar $100/mo per 50k checks** — only if we see real abuse. Skip until
  it shows up.
- **Vault** — pricing unknown. Not needed for v1; use Azure Key Vault
  directly (already in stack). Revisit if Vault offers customer-key
  segmentation for tenant-level encryption that AKV doesn't.

### Startup program

WorkOS does have a "Startup Program" / partner deal — search results
suggest ~6 months free or discounted pricing for early-stage companies.
Worth applying. Doesn't change the long-run math but extends our runway
through the prototype window.

---

## Phase 5 — Compliance / SOC 2 Posture

### What WorkOS holds

- **SOC 2 Type 2** — current, available via Trust Center.
- **GDPR** compliant; deletion on request via support.
- **CCPA** compliant.
- **Annual third-party penetration testing** + external code audits.
- **HIPAA BAA** available, but **enterprise-plan only**. Given our customer
  profile (legal, not health), we don't need a BAA at launch.

### What WorkOS does not publish

- **ISO 27001** — not mentioned on the security page. Likely not held.
- **PCI** — not mentioned. (Not load-bearing; we don't handle card data;
  Stripe does.)
- **Data residency** — no public mention of EU region pinning. WorkOS is
  US-hosted. **Open question for any future EU customer.** Likely a
  blocker if a German law firm ever shows up.
- **Subprocessor list** — exists at `trust.workos.com/subprocessors` but
  the trust portal is JS-rendered and we couldn't fetch its content via
  WebFetch. _Pull this manually before signing._

### Implications for our SOC 2 readiness

WorkOS-as-subprocessor is a clean SOC 2 story:
1. They give us SOC 2 Type 2 reports we attach to vendor risk reviews
   (Vanta/Drata flow this in automatically).
2. Their audit logs satisfy the **identity-event evidence** requirement
   (CC6.1, CC6.6) without us building it — sign-ins, MFA enrollments, SSO
   config changes are all captured.
3. Their Admin Portal becomes the customer-facing self-serve identity-
   admin tool, which is itself a SOC 2 talking point.

### Implications for the buyer's security review

**Plus.** WorkOS is well-known among engineers. Having "powered by WorkOS"
in our security page is a defensible signal at the early-deal stage.

**Minus.** Procurement at large in-house legal departments may push for
"can you support our existing IdP?" — answer: yes, via WorkOS SSO. So
this works in our favor.

**Open.** Some highly regulated customers (banks, government adjacent)
prefer first-party Okta / Entra integration. WorkOS sits in the middle
as a SAML/OIDC SP — that's typically transparent to the customer. The
risk is they ask "where does our employee data flow?" and we have to
explain WorkOS as a hop. Subprocessor disclosure handles this.

### Subprocessor-disclosure obligation on us

Adding WorkOS adds them to our public subprocessor list. Lawyers _will_
read it. Recommendation: list WorkOS as the identity subprocessor with a
one-sentence purpose ("user authentication, organization management,
SSO/SCIM"). Mirrors how every B2B SaaS handles this.

---

## Phase 6 — Audit Log Boundary

### The recommendation: WorkOS-primary, our DB mirrors

```
event source                 WorkOS audit log    our audit_events
─────────────────────────    ─────────────────   ────────────────
sign-in / MFA / SSO change   ✓ (auto)            mirror via webhook
custom app actions           ✓ (we emit)         ✓ (we write)
SCIM provisioning            ✓ (auto)            mirror via webhook
contract uploaded            ✓ (we emit)         ✓ (we write)
review run                   ✓ (we emit)         ✓ (we write)
finding exported             ✓ (we emit)         ✓ (we write)
billing change (Stripe)       —                  ✓ (we write)
plan upgraded / downgraded    —                  ✓ (we write)
admin impersonated user       —                  ✓ (we write)
```

**Why dual-write:**

1. **Latency / availability.** Our app needs to render audit history fast
   without round-tripping WorkOS for every page load. The local table is
   the read path.
2. **Joins.** Audit history joins to Matters, Reviews, Users — natural in
   Postgres, painful through an HTTP API.
3. **WorkOS export limit.** "Past 3 months exportable via CSV." If we
   need 7-year retention for compliance or customer requirement, our DB
   is authoritative.
4. **Incident isolation.** WorkOS Audit Logs has had outages
   (status.workos.com, December 2025). If their store is down, our app's
   own audit timeline must keep working.
5. **SOC 2 evidence.** Auditor wants "show me a complete log of who
   accessed Matter X." Our table is the canonical answer; WorkOS data is
   supplementary.

**Why also push to WorkOS:**

1. Customer-facing audit UI is free (WorkOS Dashboard view per Org).
2. **SIEM streaming is free** (well, $125/mo per stream, but free in
   build effort) — enterprise customers expect this and we'd otherwise
   build it ourselves with Datadog / Splunk integrations.
3. WorkOS-emitted identity events (sign-in, SSO, etc.) are richer than
   what we'd capture from middleware alone — we'd be reinventing the
   sign-in event tracking.

**Cost.** Audit log retention scales with event volume; at projected v1
volumes (~75k/mo at 500 customers) we sit comfortably below the $99/mo
floor. SIEM streaming kicks in only when an enterprise customer asks.

**Implementation note.** The dual-write should be bracketed by a single
helper:

```go
// pseudo-code
func emitAudit(ctx context.Context, e AuditEvent) error {
    // 1. Write our own row first (canonical)
    if err := db.InsertAuditEvent(ctx, e); err != nil { return err }
    // 2. Best-effort push to WorkOS; failure is logged not raised
    go workos.AuditLogs.CreateEvent(...)
    return nil
}
```

Best-effort because WorkOS being down must not block our writes.
Reconcile asynchronously if needed.

---

## Phase 7 — Risks and Lock-in

### Outage risk (real)

Per StatusGator: ~81 outages over 2 years; 6 incidents in the trailing 90
days (2 major + 4 minor); median duration ~1.5 hours. Most recent Audit
Logs outage: December 2025. Most recent Directory Sync outage: October
2025. **WorkOS being in the hot path of every login means a hard outage
= full-stop for our customers.**

**Mitigations available:**

1. **Cache the JWKS** locally so token validation survives a brief WorkOS
   API outage. (Cheap — token validation is the read path; sessions
   already issued continue to work for `exp` minutes.)
2. **Refresh-token graceful degradation.** If WorkOS is down when refresh
   is needed, hold the user's session via a longer-lived local cookie
   for up to N minutes, log the user out cleanly when WorkOS recovers
   only if no successful re-auth has happened.
3. **Status-page widget on our own status page.** When WorkOS is having
   trouble, our customers should see one truthful thing.

What we _cannot_ mitigate: new sign-ins are gated on WorkOS being up.
This is the cost of using a hosted identity provider; same is true of
Auth0, Clerk, Supabase Auth, etc.

### Lock-in risk (medium-low, industry-standard)

- Users, orgs, memberships: exportable via API.
- **Password hashes: not exportable.** Forced password reset is the only
  exit path for non-SSO users. (Not WorkOS-specific; Auth0 / Clerk same.)
- Audit logs: 3-month CSV export window via API; older data stranded.
  Mitigation in Phase 6 (mirror locally) handles this.
- SSO connections: configuration data (IdP entity ID, ACS URL, certs)
  exportable via API; the customer's IdP-side config has to be redone
  for the new provider. Realistic timeline for an SSO migration: weeks.

**Reversibility window.** The latest we could switch off WorkOS without
forcing every user to reset their password is **before any user signs up
with email+password**. After that, exit costs grow with user count. By
the time we have hundreds of paying orgs, switching is a multi-quarter
project. Plan accordingly: revisit "is WorkOS still right?" at customer
counts of ~100 (still cheap to leave) and ~500 (committed for the
duration).

### Vendor health risk (medium)

WorkOS raised a Series C in 2024, growing well, profitable enough to fund
a startup program. No acquisition rumors. The Auth0/Okta scenario — get
acquired and the product changes — is real for any identity vendor; not
WorkOS-specific. _Plan B if it happens_: Clerk for the AuthKit-like
surface, plus self-hosted SCIM/SAML server (e.g. SAML Jackson) for the
enterprise side.

### Common practical complaints (from reviews / HN)

- **Pricing on SSO is steep relative to plan tiers.** Mitigated for us by
  not offering SSO until the Large Team / Enterprise tiers.
- **Stack support beyond Next.js is "constrained."** Go SDK exists but is
  less polished than the Next.js path. Consequence for us: backend Go
  code is fine (mostly token validation + REST API calls); frontend in
  React+Vite-or-similar will work but lacks the Next.js "drop-in" magic.
  _We probably want to use the AuthKit-React component library and
  manage the redirect-to-callback dance ourselves._
- **Free tier "doesn't show off the value."** True; the value is the
  enterprise feature set, which is paid. Acceptable; we plan to use it.
- **No mobile SDKs (iOS/Android).** Not a concern — we're web-first.

---

## Phase 8 — Stripe Integration: Sample Flows

### Signup with plan choice and payment (solo case)

```
[Marketing site /pricing]
        │
        │ click "Sign up — Pro plan"
        ▼
[/signup?plan=pro]  ──[1]── AuthKit hosted /Account UI for email+password
                                     or  Google OAuth
        │
        │ AuthKit posts user back to our callback with code
        ▼
[/auth/callback]  ──[2]── exchange code → access+refresh tokens
                            user_id, email known; org_id absent
        │
        ▼
[server] ──[3]── Create WorkOS Organization (name = "<email>'s workspace",
                                             external_id = our_org_uuid,
                                             metadata.is_solo = "true")
         ──[4]── Create OrganizationMembership (user, org, role=owner)
         ──[5]── Create Stripe Customer (email, metadata.workos_org_id)
         ──[6]── Update WorkOS Org with stripeCustomerId
         ──[7]── Create Department row in our DB (org_id, name="Default")
         ──[8]── Re-authenticate session into the new org
                  (authenticateWithRefreshToken with org_id arg →
                   new access token containing org_id, role, permissions)
        │
        ▼
[/checkout]  ──[9]── Stripe Checkout session for plan=pro, $200/mo
        │
        │ Stripe redirects on success → our /post-checkout
        ▼
[/post-checkout]  ──[10]── Stripe webhook (subscription.created)
                            → flip our DB tier="pro_active",
                              record current_period_end, etc.
        │
        ▼
[/dashboard]   first session in real product
```

**Steps 3–8 are the gnarly bit:** we're orchestrating WorkOS, Stripe, and
our own DB in a single signup. Idempotency keys on every external call,
and a saga-pattern with cleanup if any step fails. The
`next-b2b-starter-kit` reference repo demonstrates a clean version of
this for Next.js + Convex; same shape applies to Go.

**Critical detail:** WorkOS does NOT subscribe to Stripe webhooks for us.
Subscription state ↔ entitlement is our responsibility. The Stripe
add-on _does_ thread Stripe entitlements into the access token (via JWT
claims at refresh time), but only if we wire it up.

### Cancellation

```
[user] ──[1]── /account → "Cancel subscription" button
        ▼
[server] ──[2]── Stripe API: subscription.cancel_at_period_end = true
         ──[3]── audit_event: "subscription.cancellation_scheduled"
        │
        ▼ ... days/weeks later, period end ...
[Stripe webhook: customer.subscription.deleted]
        ▼
[server] ──[4]── flip our DB tier="cancelled"
         ──[5]── revoke API access (or downgrade to read-only)
         ──[6]── audit_event: "subscription.terminated"
         ──[7]── schedule data-retention job (delete in 30/60 days)
```

WorkOS state on cancellation:
- Org and Memberships **stay active** in WorkOS until our retention timer
  fires; the user can still log in and view their data during the
  grace period.
- After grace: deactivate Memberships (`status = inactive`), eventually
  delete Org and User if the user requested account deletion.
- Memberships keep audit history attached even if deactivated. We do NOT
  delete by default — we deactivate.

### Hard account deletion (GDPR / explicit user request)

1. User initiates "delete my account" in /account.
2. Confirm via email link (24-hour window, audit-logged).
3. Run deletion job:
   - Delete Matters, Reviews, Findings (cascade) from our DB.
   - Delete Blob Storage objects for that org.
   - Delete WorkOS Memberships, then User (or Org if last user).
   - Audit log a final "account.deleted" event from a system actor.
4. Retain anonymized usage_events / billing rows for legal / financial
   record-keeping per our retention policy.

---

## Phase 9 — App Integration: Core Flows

### Login

1. User hits `/login` → frontend redirects to AuthKit hosted login page
   (cleanest) OR renders an `@workos-inc/authkit-react` component
   (Tom's preference). Both supported.
2. AuthKit handles email+password / magic auth / Google / SSO.
3. On success, AuthKit POSTs to our `/auth/callback` with a code.
4. Server exchanges the code via `userManagement.authenticateWithCode()`
   → returns access token (JWT) + refresh token + user.
5. Server selects the user's Org (single Org for solos; show selector if
   multi-Org — punt on multi-Org for now per Tom's note on Q3).
6. Server calls `authenticateWithRefreshToken(refresh_token, org_id)`
   → returns new access token with `org_id` and `role` claims populated.
7. Server sets a secure, httpOnly cookie holding the access token (and
   refresh token in a backend session store or sibling cookie).
8. Frontend redirects to `/dashboard`.

### Signup

Same as login but the AuthKit form has email+password creation. On
callback, the access token has no `org_id`. We branch:

- **Solo flow.** As described in Phase 8 (auto-create Org, Dept, Stripe
  Customer, push user through Stripe Checkout).
- **Invited-user flow.** Token has `pending_authentication_token` claim
  (or invite-id query param). Server reads invite, finds the
  pre-existing Org, accepts the invitation, creates the
  OrganizationMembership, redirects user into the existing Org.
- **Self-serve team flow.** Punt to v2; route through marketing-site
  contact form for now.

### Signout

1. User clicks Sign Out.
2. Frontend POSTs `/auth/signout`.
3. Server: extract `sid` from the validated JWT, call WorkOS `/logout`
   endpoint with it. WorkOS revokes the session at the source. We also
   clear our local cookies.
4. Audit event written: `session.revoked` (matches WorkOS's webhook event
   name for symmetry).
5. Frontend redirects to `/`.

**Note:** there is no WorkOS-native "log out everywhere." If we want it,
we track all active `sid`s for a user in our DB and call `/logout` for
each. Reasonable v2 feature; not v1.

### Cancellation / account termination

See Phase 8 above.

---

## Phase 10 — Promote-Org Path Design Sketch

Pre-condition: solo Org exists with `is_solo = true`, one User, one
Membership (role=owner), one Department ("Default").

Trigger options:
1. Solo clicks "Invite teammate" in Account → Members.
2. Solo upgrades to Small Team in Account → Plan.
3. Either path implicitly promotes.

Flow:
```
[/account/members → Invite]
       │
       ▼
[server] ─ require plan upgrade if currently solo
         → if plan != team-tier, route through Stripe to upgrade subscription
         → on Stripe webhook for subscription updated:
              flip our DB: tier = "team", is_solo = false
              prompt user to set Org name and Dept name
              reveal team UX (members panel, invite form, etc.)
       │
       ▼
[user fills invite form: email + role + department]
       │
       ▼
[server] ─ WorkOS: invitations.create({ email, organization_id, role })
         WorkOS sends email. Accept link returns to /auth/callback.
         New User signs up via AuthKit (or signs in if existing).
         WorkOS creates OrganizationMembership.
         We listen to the organization_membership.created webhook,
         attach a department_id (from the invite metadata) in our DB.
       │
       ▼
[/dashboard]  multi-user team UX live. No data migration. Same Org id.
```

**Key invariants preserved:**
- The Organization id never changes. All Matters stay attached.
- The original solo's User id never changes. Their audit history stays.
- The Department id never changes. Default Dept becomes a real Dept.
- Stripe Customer id never changes. Subscription is upgraded in place.

**Edge cases:**
- Solo invites a teammate without doing the plan upgrade first: server
  blocks the invite send and routes through plan upgrade first.
  (Don't let the team UX exist without the team plan.)
- Solo wants two Departments. Allow renaming "Default" and creating
  additional Departments via Account → Departments. Visible only after
  promotion.
- Solo cancels Team plan. Downgrade is messy (more users than the solo
  plan's seat = 1 allows). Force the user to deactivate other memberships
  before downgrade is allowed. (Punt detailed UX to v2.)

---

## Phase 11 — Sample SSO Integration Walkthrough

Customer scenario: a 60-lawyer firm wants to use their Okta IdP to log
into Accordli.

**Step 0.** Customer is already a paying Accordli customer (Large Team or
Enterprise). They have an Org, a few Departments, a few users. This
flow upgrades their auth to SSO.

**Step 1 (Accordli internal).** Sales/CS rep uses our admin tool to
generate an Admin Portal setup link for that customer's WorkOS Org. The
backend call is `portal.generateLink({ organization, intent: 'sso' })`.
This returns a URL like
`https://your-app.workos.com/admin-portal/launch?token=...`.

**Step 2.** We email the link to the customer's IT admin. (Or WorkOS
emails it on our behalf — toggle via the call options.)

**Step 3 (Customer IT admin).** Clicks the link. Lands in WorkOS-hosted,
Accordli-branded Admin Portal. Sees:
- Step-by-step instructions for Okta SAML.
- "Add this app to your Okta tenant; here's the Entity ID, ACS URL,
  signing cert."
- Form fields to paste back: Okta IdP metadata XML (or URL), Okta
  signing cert, attribute mappings.
- "Test connection" button using the Test IdP.

**Step 4.** IT admin completes Okta-side: creates SAML app in Okta admin,
copies metadata back, pastes into Admin Portal. Hits "Activate."

**Step 5.** WorkOS connection state flips to `active`. Webhook
`connection.activated` fires to our backend → we mirror the connection
state in our DB if we want to surface it in our UI.

**Step 6.** Domain Verification (parallel or before Step 5). Customer
adds CNAME or TXT records on `lawfirm.com`. WorkOS confirms ownership.
After this, AuthKit can route `@lawfirm.com` users automatically to
the SAML connection (no email/password fallback for that domain).

**Step 7.** First test login. A lawyer at the firm visits
`https://app.accordli.com/login` (or our auth subdomain). Enters
`alice@lawfirm.com`. AuthKit sees the verified domain, redirects to
Okta. Okta authenticates, redirects to WorkOS ACS, WorkOS issues a JWT
and sends back to our callback. We call
`authenticateWithRefreshToken(..., org_id=<lawfirm_org_id>)` and the
user is in. If a Membership doesn't exist yet, we create one (or apply
SCIM-driven provisioning if SCIM is also configured).

**Step 8 (optional).** Repeat for SCIM (Directory Sync) — same Admin
Portal, separate connection. SCIM events fire as users are added/
removed in Okta; we mirror to our DB and to memberships.

**Total time on the customer's side:** typically 30–60 minutes for an
experienced IT admin who has done SAML before. Test IdP first while
internal, then switch to real IdP without changing app code.

**Total work on Accordli's side (one-time):** maybe a week of
engineering to wire the Admin Portal launch endpoint, the connection-
state webhook handler, the SCIM membership webhook handler, and the
customer-facing "SSO is enabled" indicator in our app. Most is reading
events and updating local rows.

---

## Phase 12 — Recommendation

### Adopt WorkOS for v1, with these constraints

**Why yes.** WorkOS gives us, for free up to 1M MAU, the entire
"AuthKit-equivalent UX, Org/Member CRUD, password+passkey+social+magic-
auth, MFA, invitations, hosted login, JWT-based sessions" surface that
would otherwise consume engineer-months. The B2B identity primitives
(Org, Membership, Org-scoped roles, Admin Portal, audit logs, SIEM
streaming) line up with our customer profile better than any other
vendor's. The lock-in is industry-standard (no password export). The
SOC 2 Type 2 + clean audit log story shortens our compliance path.

**Why constrained:**

1. **Use it as the auth + identity substrate, not as the application
   data model.** Our DB stays canonical for Org, User, Membership,
   Department, Matter, Review, Finding, audit log. WorkOS is mirrored
   in. Use `external_id` to pin our uuids to their objects.
2. **Department lives in our DB**, mirrored to WorkOS user metadata
   (`accordli_dept_id`) and promoted to a JWT custom claim via a JWT
   template. API middleware reads `dept_id` off the validated JWT — no
   Postgres hit on the read path. No FGA in v1; plain `dept_id` checks
   are enough until question 10 becomes real.
3. **SSO and Directory Sync: gate behind Large Team and Enterprise
   tiers only.** Per-connection cost is too high for Small Team
   margins.
4. **Audit logs: dual-write.** Our DB is canonical for the read path
   and long-term retention; WorkOS gets a best-effort copy for the
   customer-facing dashboard view, SIEM streaming, and identity-event
   capture. Helper function in Go bracket-writes both.
5. **Stripe orchestration: ours.** WorkOS provides a clean Org ↔ Stripe
   Customer link via `stripeCustomerId`; everything else (subscription
   webhooks, plan changes, cancellation flows) is in our Go code.
6. **Materialize the (default) Org and Department on day one for solos**
   so the promote-org path is a flag-flip, not a data migration.
7. **Cache JWKS locally** so brief WorkOS API outages don't break
   already-issued sessions.

### Open items to resolve before committing

- [ ] Pull WorkOS subprocessor list manually from `trust.workos.com/subprocessors`
      (Cloudflare confirmed, others unconfirmed).
- [ ] Confirm Vault, FGA, Admin Portal, MFA pricing with sales (none on
      public page). Specifically: is Admin Portal free with paid SSO?
- [ ] Confirm Audit Log retention duration (the 3-month figure is the
      _export_ window; not necessarily total retention).
- [ ] Apply to the WorkOS Startup Program; quantify the deal.
- [ ] Decide between hosted AuthKit UI vs. `@workos-inc/authkit-react`
      embedded — Tom's preference is React components. Spike both for
      a day each during prototype window and pick.
- [ ] EU residency: explicitly punt unless an EU customer asks. Note
      in `notes/todo.md` for revisit.

### Decision review checkpoints

- **At ~100 paying orgs.** Re-run the pricing math; confirm SSO/SCIM
  margin still works at the actual adoption shape. Revisit "is WorkOS
  the right substrate?" with low exit cost.
- **At ~500 paying orgs or first SOC 2 Type II audit.** Lock-in is now
  meaningful. Re-confirm vendor health, evaluate any Plan B candidates
  (Clerk maturity, Ory enterprise tier).

### What this changes in current specs

- `notes/contract-ai-saas-roadmap.md` — already lists WorkOS; tighten
  the Tenancy and Auth Glue section with the dual-write audit pattern
  and the external_id mapping.
- `notes/product-specs/accordli_platform_overview.md` — add a sentence
  to the "Department lives in our DB" decision so future readers don't
  ask.
- `notes/todo.md` — close item 15 ("WorkOS deep dive") with a pointer
  here. Add the open items above as new sub-items.

---

## Phase 13 — Alternatives Considered: Entra External ID

Since the cloud commitment is Azure, the natural question is "why not
Microsoft's customer-identity product?" The right comparison is
**Entra External ID** (formerly Azure AD B2C, rebranded and unified
in 2024) — Microsoft's CIAM offering. Plain Entra ID is the
workforce/employee IdP, used internally; it is not a customer-facing
auth substrate.

### Pro/con table

| Dimension | WorkOS | Entra External ID |
|---|---|---|
| B2B-SaaS data model | Native Org + Membership + per-Org SSO connection | "Users in a directory"; multi-tenant SaaS shape requires modeling on top |
| Developer experience | Clean docs, tight SDKs, drop-in React components | Sprawling docs, heavier SDKs, branding via user flows or XML custom policies |
| Time to ship MVP auth | Days | Weeks to months for equivalent depth |
| Customer SSO self-service | Admin Portal (turnkey, branded) | Build on Microsoft Graph APIs |
| SCIM directory sync | 12+ IdP connectors maintained by WorkOS | SCIM-capable; less polished ecosystem |
| Audit log → customer SIEM | 6 destinations as a checkbox | Pipe via Log Analytics → Event Hubs → customer SIEM |
| Pricing ≤1M MAU | AuthKit free; SSO/SCIM at $125/mo each | 50K MAU free; then $3.25/1K MAU; SSO/SCIM bundled |
| Pricing at 100K+ MAU | $2,500/mo per additional 1M + per-connection | ~$162/mo at 100K — materially cheaper at scale |
| Per-SSO-customer cost | $125–250/mo per customer | Bundled — better unit economics on SSO-dense books |
| In our cloud already | New DPA, new subprocessor | Same tenant, same billing, MACC-eligible |
| Compliance posture | SOC 2 Type 2, GDPR, CCPA; HIPAA BAA enterprise-only; no public ISO 27001 | SOC 2, ISO 27001, FedRAMP High, HIPAA, GDPR, EU residency |
| Lawyer-buyer brand | Recognized by engineers; lawyers don't know it | "Powered by Microsoft" is universally recognized |
| EU residency | Not published | Native EU region support |
| Support | Small, responsive | Microsoft enterprise support — slow for non-enterprise tiers |
| Lock-in | Password hashes non-exportable | Same |
| B2B cross-tenant federation | None | Genuine federation with customer Entra tenants |

### Headline finding

The decision is dominated by **developer velocity and B2B-SaaS-shape
ergonomics**, not feature parity. Entra External ID is feature-complete
on paper for our use case; the cost is engineering time to fit a
multi-tenant SaaS shape onto a "users in a directory" substrate, plus
custom-built equivalents of WorkOS's Admin Portal, SCIM connector
ecosystem, and SIEM-streaming UX.

### Decision frame

| If this is true | Then | Why |
|---|---|---|
| 2 engineers, building toward solo + small team launch in 2026 | **WorkOS** | Velocity wins; the SSO-tax problem is solved by gating SSO behind Large Team |
| 5+ engineers, BigLaw / federal / heavy Microsoft-shop customer base | Reconsider | Entra External ID's bundled SSO + Microsoft brand may justify integration cost |
| MAU > 100K | Reconsider | Per-MAU economics flip dramatically |
| EU residency required by a real customer | Reconsider | WorkOS does not publish EU pinning; Microsoft does |

For us, today, **WorkOS is the right call.** The
decision-review checkpoints already in Phase 12 (~100 paying orgs;
~500 paying orgs / first SOC 2 Type II audit) are the natural points
to re-pull this comparison.


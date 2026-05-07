# WorkOS ↔ SoloMocky integration plan

Design for sign-up / sign-in / sign-out using WorkOS AuthKit as the hosted IdP and keeping our four identity tables (`organizations`, `departments`, `users`, `memberships`) in sync. Built on top of the existing `prototypes/workos-prototype` learnings; adapted to the `infra/auth` seam and the actual SoloMocky schema in `db/schema.sql`.

---

## 1. Architectural choices

### 1a. Three sync paths, with the OAuth callback as the fast path

Terminology first, because it matters for the rest of the doc:

- **`/auth/callback`** — the **OAuth redirect URI** the user's *browser* hits after AuthKit finishes login. Synchronous in the request that's about to redirect to the FE. This is where we do DB writes that must be visible before the user lands on the app.
- **`/webhooks/workos`** — the unrelated server-to-server notification WorkOS sends us out-of-band. **No DB writes here**, ever; it's a wake-up nudge for the events drain.

These are different endpoints, different trust models, and different lifecycles. Don't conflate them.

WorkOS exposes two ways to learn that something changed: **webhooks** (push) and the **Events API** (pull, cursor-based). Their own docs explicitly recommend the Events API for keeping a local DB in sync, because:

- Cursored, in-order, replayable for up to 90 days.
- One worker per cursor → trivial dedup, no thundering herd, no webhook signature theatre on every retry.
- Webhook endpoint can stay a thin "wake-up nudge" — no business logic in the HTTP handler.

But the Events API has latency. A user who signs up and immediately lands on `/auth/callback` will be at the app **before** any event is drained. We can't show "Hi <email>" if the row doesn't exist yet.

So we run **three** code paths, in priority order:

1. **Lazy upsert at `/auth/callback`** — the response from `AuthenticateWithCode` already contains the user, the org, and (after the membership lookup the prototype already does) the membership. We have everything we need to create or update our local rows synchronously, before redirecting to the FE. This is the latency-critical path. **It runs identically on signup and signin** — signup creates rows, signin updates them. (Crucial for catching dashboard-side role flips between sessions; see §3.)
2. **Events API drain worker** — a long-running goroutine in the API process that pulls events since `cursor`, applies them idempotently, advances `cursor`. This is the **canonical** sync and is what catches every change made in the WorkOS dashboard, by an admin, or by a future SCIM connector — i.e. things the user's browser will never see.
3. **Webhook endpoint** — already wired as a placeholder at `/webhooks/workos`. After Phase 1 it verifies the signature, then triggers the drain worker to wake up early instead of waiting for the next tick. **No DB writes happen in the webhook handler.** It's a notification, nothing more.

This split — callback for latency, events for correctness, webhooks as an optional nudge — means we can start with just (1) and (2), defer signature verification to whenever we feel like, and never have to debug an out-of-order webhook.

### 1b. Departments are local-only

WorkOS has no concept of a Department, and we shouldn't try to fake one in their custom-attributes blob. Departments are an internal product abstraction that exists for solo-vs-firm UX and for matter scoping. Therefore:

- **Every WorkOS org gets exactly one Department on creation**, named `"{first} {last}'s Team"` (with the same email-fallback rule as the org name), `is_default=true`. No round-trip to WorkOS.
- **Solo orgs (`is_solo=true`)** hide the department picker entirely — the default dept is the only dept and the UI pretends it isn't there.
- **Memberships always carry a `department_id`.** New memberships default to the org's default dept. If a firm later splits into Litigation / M&A / etc., we add departments locally and reassign memberships in our app — WorkOS never knows.

The schema already supports this: `departments.organization_id`, `departments.is_default`, `memberships.department_id`. No schema change.

### 1c. Soft delete, not hard delete

The schema already has `users.deleted_at`, `organizations.deleted_at`, and `matters.deleted_at`. `memberships` has `status` (default `'active'`). When WorkOS sends a `*.deleted` event:

- `user.deleted` → `UPDATE users SET deleted_at = now()`. Don't cascade — Matters they authored remain.
- `organization.deleted` → `UPDATE organizations SET deleted_at = now()`, soft-delete its default department. Don't touch matters yet — that's a separate retention-policy decision.
- `organization_membership.deleted` → `UPDATE memberships SET status = 'inactive'`. Keep the row for audit.

This is the only safe default for a legal product where customers expect to be able to recover an "I deleted that yesterday" matter.

### 1d. Prefixed IDs for identity tables; native UUIDs for everything else

We adopt a **hybrid ID scheme**: tables whose identifiers cross our process boundary (echoed back by WorkOS, stamped on Stripe metadata, attached to Helicone traces, pasted into support tickets) get human-typeable prefixed IDs stored as `text`. Tables that are purely internal keep native `uuid`. Concretely:

| Table | ID type | Form | Rationale |
|---|---|---|---|
| `organizations` | `text` | `ao_<base62-uuidv7>` | echoed by WorkOS, Stripe, every external surface |
| `departments` | `text` | `ad_…` | will be echoed by Stripe (per-dept usage), referenced in URLs |
| `users` | `text` | `au_…` | echoed by WorkOS, in support tickets |
| `memberships` | `text` | `am_…` | echoed by WorkOS in `organization_membership.*` events |
| `matters`, `documents`, `review_runs`, `lens_runs`, `findings` | `uuid` | bare UUIDv7 | internal only; no external system sees them |

Why hybrid:

- **Where prefixes pay**: identity rows are low-volume (thousands today, low millions ever) and high-cross-reference. Having `ao_2gT9pFXk…` show up identically in a WorkOS event payload, our DB dump, the Stripe customer-metadata field, a Helicone trace tag, and a Linear ticket eliminates an entire class of "is this the same record?" friction. The +11–26 bytes per row is invisible at this scale.
- **Where prefixes don't pay**: `findings` will be the largest table eventually (tens of millions of rows in a few years). Native `uuid` keeps that index tight and `shared_buffers`-friendly. Findings IDs are never seen outside the app — no benefit to making them human-typeable.

**FK boundary.** Internal tables that reference identity tables now carry `text` columns: `matters.organization_id`, `matters.department_id`, `matters.created_by_user_id`, `review_runs.organization_id`, `lens_runs.organization_id`, `findings.organization_id`. The PKs on those internal tables stay `uuid`. Mildly odd visually (a row has a `uuid` PK and a `text` FK) but mechanically fine — `text` FKs index and join the same as any other column.

**ID generation.** A small `internal/ids` package owns the codec:

- `ids.NewOrg() string` returns `ao_<base62(uuidv7())>`.
- `ids.Parse(s) (kind, uuid.UUID, error)` validates prefix and decodes — used at API boundaries.
- Generation is in Go, never `DEFAULT` in the DB. Per CLAUDE.md, pre-mint before INSERT so the row can self-reference.

**Suffix encoding: base62, not hex.** Base62 of a uuidv7 preserves byte-order (so B-tree locality from uuidv7 carries through), and it's ~22 chars vs hex's 36 — `ao_2gT9pFXk3WqRzL5BcDeFgH` reads cleaner than `ao_018f4c2e-1a2b-7d3e-9f4a-1c2b3d4e5f60`.

**`external_id` round-trip with WorkOS.** WorkOS exposes an `external_id` field on User, Organization, and Membership — a unique, free-form string we set at create time. We pass our `ao_…` / `au_…` / `am_…` directly as `external_id`. Now every WorkOS event, webhook, and dashboard search carries our exact ID, prefix and all. Reverse lookup is a single column read on either side. Three identifiers each pull their weight:

| Column | Set by | Used for |
|---|---|---|
| `organizations.id` (`text`, `ao_…`) | us, pre-INSERT | local FKs, app code, all external surfaces |
| `organizations.workos_org_id` (`org_…`) | WorkOS, returned at create | every WorkOS API call we make |
| `external_id` on the WorkOS side | us, = our `ao_…` | reverse lookup; dashboard search by our ID |

Names — both ours and WorkOS's — are descriptive labels, not keys. WorkOS docs are explicit: an organization name "does not need to be unique." Don't ever look up by name.

---

## 2. Event → table mapping

WorkOS emits both User Management events (the AuthKit ones we care about) and Directory Sync events (`dsync.*`, only relevant once a customer wires SAML/SCIM — Phase post-Mocky). The User Management set we handle in SoloMocky:

| WorkOS event | Local action | Notes |
|---|---|---|
| `organization.created` | upsert `organizations` by `workos_org_id`; if newly inserted, also insert one `departments` row with `is_default=true` | First time an org appears we synthesize its default dept atomically in the same txn. |
| `organization.updated` | `UPDATE organizations SET name=…, metadata=… WHERE workos_org_id=…` | |
| `organization.deleted` | `UPDATE organizations SET deleted_at=now()` and soft-delete dept | |
| `user.created` | upsert `users` by `workos_user_id` | `email` from payload, `current_dept_id` left NULL until a membership exists. |
| `user.updated` | `UPDATE users SET email=…` | Keep the local PK stable; never touch `id`. |
| `user.deleted` | `UPDATE users SET deleted_at=now()` | |
| `organization_membership.created` | upsert `memberships` by `workos_membership_id`; set `department_id = default_dept_of(organization_id)`; set `users.current_dept_id` if NULL | This is the join row WorkOS doesn't have a great mental model for; for us it's the unit of access. |
| `organization_membership.updated` | `UPDATE memberships SET role=…, status=…` | Don't reassign `department_id` from a WorkOS event; that's user-driven. |
| `organization_membership.deleted` | `UPDATE memberships SET status='inactive'` | |
| `session.created` | no-op (today) | Future: audit trail table; useful for "show me who's logged in from where" admin UI and SOC 2. |
| `session.revoked` | no-op (today) | Same. |
| `authentication.*_succeeded` / `*_failed` | no-op (today) | These become inputs to a security-events stream when we get to monitoring. Don't write to identity tables from these. |
| `invitation.*` | defer | Not part of SoloMocky. Re-evaluate when a firm-tier flow exists. |
| `dsync.*` | defer | Not until SCIM customers exist. |

**Idempotency.** Every handler is `INSERT … ON CONFLICT (workos_*_id) DO UPDATE …` keyed by the WorkOS ID. Replay-safe by construction. Where the schema doesn't already have a uniqueness constraint, the existing `UNIQUE` on `workos_org_id`, `workos_user_id`, `workos_membership_id` carries us — nothing to add.

**Partial updates: WorkOS-owned columns overwrite, local-only columns don't.** Per row:

- `memberships`: WorkOS owns `role`, `status`. **Local owns `department_id`.** A signin upsert must never reset `department_id` — if a user moved themselves from Default → Litigation in our app yesterday, a signin today shouldn't undo that. The `ON CONFLICT DO UPDATE … SET` clause lists only WorkOS-owned columns.
- `users`: WorkOS owns `email`. Local owns `current_dept_id`. Note `email` is just a denormalized cache; the join key is `workos_user_id`, never `email`. (WorkOS lets users change emails; if we keyed off email we'd get phantom-row bugs.)
- `organizations`: WorkOS owns `name`. Local owns `is_solo`, `tier`, `billing_status`, and most of `metadata` (Stripe writes some metadata fields; coordinate when Stripe lands).

This rule is the same in the callback path and the events-drain path.

**Freshest-wins between callback and drain.** If both fire for the same row near-simultaneously, the events-drain handler compares the event's `updated_at` against the local row's `updated_at` and skips if stale (WorkOS docs explicitly recommend this pattern). The callback can write unconditionally — it's pulling live API state, which by definition is the newest WorkOS knows. The drain's stale-skip is what protects against an out-of-order older event clobbering the callback's fresh write.

**Ordering.** Events are chronological per stream. Cross-stream isn't strictly guaranteed (a `membership.created` could in principle reach us before its `user.created`). Mitigation: if the FK lookup fails inside an event handler, fall back to a single `usermanagement.GetUser` / `GetOrganization` call to fetch the parent and upsert it, then proceed. Don't drop the event; don't advance the cursor past it. The retry on the next drain tick will pick it up cleanly.

---

## 3. Sign-up, sign-in, sign-out flows

### Sign-up (solo, first time we see a user)

We **don't** use AuthKit's hosted "Create your team" page — wrong UX for a solo lawyer who doesn't think of themselves as a team. AuthKit is configured for user-only signup; the org gets created server-side by us in the callback.

1. FE button hits `/auth/login`. AuthKit collects email+password / Google, creates the WorkOS user, bounces back to `/auth/callback?code=…`. **No org created yet.**
2. Callback exchanges code via `AuthenticateWithCode`. Response gives us `User` + `AccessToken` + `RefreshToken`; `OrganizationID` is empty.
3. Detect the no-org case (empty `OrganizationID` *and* `ListOrganizationMemberships(UserID)` returns zero rows) → first-time signup.
4. **Pre-mint local IDs** in Go using the `ids` package: `ao_…` for the org, `ad_…` for the default dept, `au_…` for the user (if not already present), `am_…` for the membership.
5. **Create the org server-side** with two WorkOS API calls, in order:
   - `organizations.CreateOrganization{Name: derivedName, ExternalID: aoID}` → returns the WorkOS `org_…` ID. (Our `ao_…` is now stamped on the WorkOS row's `external_id` and will echo back in every event.)
   - `usermanagement.CreateOrganizationMembership{UserID: resp.User.ID, OrganizationID: orgID, RoleSlug: "admin", ExternalID: amID}` → returns membership.
6. **Re-mint an org-scoped session.** The token from step 2 is unscoped. Call `AuthenticateWithRefreshToken{RefreshToken, OrganizationID: orgID}` to get a token whose claims carry `org_id` and `role=admin`. Use *that* for the cookie.
7. **Lazy-upsert locally** in one DB txn: `organizations` row (`is_solo=true`, `tier='solo'`, our pre-minted UUID), default `departments` row, `users` row, `memberships` row pointing at the default dept.
8. Seal the session cookie and redirect to FE.

**Org and default-department name derivation:**
- Prefer `"{first} {last}'s Organization"` for the org and `"{first} {last}'s Team"` for the default department, if first/last are present from Google or the signup form.
- Else `"{email}'s Organization"` / `"{email}'s Team"` (e.g. `"jane@law.example's Organization"`) as a placeholder, with a "Rename" prompt the first time they hit settings.
- **Don't** derive from email domain — solo Gmail users would all become `"Gmail"`.
- Names are not unique on the WorkOS side anyway, and we never look up by name, so collisions don't matter.

**Failure between 5a and 5b.** If `CreateOrganization` succeeds but `CreateOrganizationMembership` fails (network blip), there's now an orphan WorkOS org with no member. SoloMocky mitigation: wrap step 5 in a retry-with-backoff. Orphan-org reconciliation (look up by `external_id`, reuse or delete-and-recreate) is a TODO that belongs in the firm-flow phase.

Some seconds/minutes later, the events-drain pulls `user.created` + `organization.created` + `organization_membership.created` for what we just made; the upserts no-op because the rows are already there with current `updated_at`.

### Sign-in (returning user)

Same callback, same lazy upsert — but the upsert path runs against existing rows, applying only WorkOS-owned columns (§2 partial-update rule).

The reason this matters: between signup and a signin three days later, an admin may have flipped the user's role in the WorkOS dashboard. The events drain would catch it eventually, but the user has already arrived at the app. Running the upsert in the signin path is what gives them the new role on the first request, instead of waiting up to a drain interval.

Crucially, `department_id`, `current_dept_id`, `is_solo`, `tier` etc. — the local-only columns — are *not* in the upsert's `SET` clause, so signin can never undo a local change.

### Sign-out

Two layers:

- **Local sign-out** (`/auth/logout`): clears our session cookie. Cheap, immediate, what the prototype does today.
- **Full sign-out**: redirect the browser to WorkOS's hosted logout URL. Necessary if the customer expects "sign out everywhere", and necessary for shared-device scenarios where AuthKit's own session would otherwise auto-resume.

Default for SoloMocky: local-only. Add a "Sign out everywhere" link later for compliance/UX. We do not need any DB writes on sign-out — `session.revoked` is currently a no-op.

---

## 4. Cursor and worker

A single new tiny table (drop into `db/schema.sql`, no migration per CLAUDE.md):

```sql
CREATE TABLE workos_event_cursor (
    singleton  BOOLEAN PRIMARY KEY DEFAULT TRUE,
    cursor     TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (singleton = TRUE)
);
INSERT INTO workos_event_cursor (singleton, cursor) VALUES (TRUE, NULL);
```

The `singleton = TRUE PRIMARY KEY` + `CHECK` is the standard one-row-table trick — guarantees we can only ever have one cursor.

Worker loop, in process (today's seam choice — Goroutine queue. River-on-Postgres later, but a polling cursor doesn't need a queue at all, just a ticker):

```
every 30s (or on webhook-triggered wakeup):
  read cursor
  page = workos.ListEvents(after=cursor, limit=100)
  for each event in page (in order):
    handler[event.event].Apply(event.data)   // idempotent upsert
  if page non-empty:
    UPDATE workos_event_cursor SET cursor = page.last_id, updated_at = now()
  if page.has_more: loop again immediately
```

A single goroutine. No fan-out — keep it boring. The events-drain handler imports `infra/repo`; it does not depend on `httpapi`.

**Backfill at first deploy:** WorkOS retains 90 days. On first run, with `cursor = NULL`, the API returns the most recent page. Either accept that (you don't care about events from before you turned the worker on) or pass `range_start = some_explicit_timestamp` to bootstrap. SoloMocky: accept it; the `/auth/callback` lazy-upsert covers any user who shows up in the meantime.

---

## 5. Phasing

Roughly aligned with the existing Phase 0 / Mocky / Analyze breakdown:

- **Phase 0a (this work)** — replace `auth.NewHardcoded` with a `auth.NewWorkOS` provider behind the same `auth.Provider` interface. `Resolve(ctx, r) → *Identity` reads our cookie, validates, hits local `users` + `memberships` tables for the org/dept/role. No external WorkOS call on the hot path.
- **Phase 0a (cont.)** — add `/auth/login`, `/auth/callback`, `/auth/logout` to the chi router as public endpoints. Port the prototype's session sealing into `infra/auth`.
- **Phase 0b** — events-drain goroutine + cursor table. Lazy-upsert in the callback.
- **Phase 0c** — webhook signature verification at `/webhooks/workos`, body-discarded, just calls `worker.Wake()`.
- **Phase 1+** — replace the goroutine with a River job if/when we want retries with jitter, dead-letter, etc. Until then, a ticker is fine.

---

## 6. What this changes in the existing code

Concrete touch list, so the diff is predictable:

- `api/internal/infra/auth/` — add `workos.go` next to `hardcoded.go`. Both implement `Provider`. `cmd/api/main.go` switches on env (or build tag) to pick which one to wire.
- `api/internal/httpapi/router.go` — add public routes `/auth/login`, `/auth/callback`, `/auth/logout`. Keep them outside the `authMiddleware` group.
- `api/internal/httpapi/auth.go` (new) — handlers, structurally a copy of `prototypes/workos-prototype/be/internal/handlers/auth.go` adapted to chi + the seam.
- `api/internal/infra/repo/` — new `IdentityRepo` methods: `UpsertOrganizationByWorkOSID`, `UpsertUserByWorkOSID`, `UpsertMembershipByWorkOSID`, `EnsureDefaultDepartment`, `MarkOrgDeleted`, `MarkUserDeleted`, `MarkMembershipInactive`. Every method takes `org_id` already because of CLAUDE.md's multi-tenant scoping rule (the upsert-by-WorkOS-id ones are unusual in that they discover the `org_id` rather than receive it — call out in the comment).
- `api/internal/core/identity/` (new) — events-drain orchestrator. Owns the cursor, the ticker, the dispatcher mapping event type → repo method.
- `api/internal/ids/` (new) — ID codec. `NewOrg() / NewDept() / NewUser() / NewMembership()` constructors and a `Parse(s) (kind, uuid.UUID, error)` decoder. One-screen file; tests verify round-trip and prefix validation.
- `api/internal/httpapi/webhooks.go` — keep the placeholder shape; later add HMAC verification and a `worker.Wake()` call.
- `db/schema.sql` — change `organizations.id`, `departments.id`, `users.id`, `memberships.id` to `text`. Update every FK that references them (`departments.organization_id`, `users.current_dept_id`, `memberships.user_id`/`organization_id`/`department_id`, `matters.organization_id`/`department_id`/`created_by_user_id`, `review_runs.organization_id`, `lens_runs.organization_id`, `findings.organization_id`) to `text`. Drop `DEFAULT gen_random_uuid()` on the four identity tables — IDs come from Go. Internal tables (`matters`, `documents`, `review_runs`, `lens_runs`, `findings`) keep `uuid` PKs. Add `workos_event_cursor` table.
- `web/src/api.ts` — add a `me()` call. Replace the assumption of a hardcoded identity in `MatterList`/`MatterDetail` with a top-level "are we signed in?" check; redirect to `/auth/login` on 401.
- `api/internal/solomocky/hardcoded.go` — kept until the cutover env var flips. The Org/Dept/User UUIDs in there are fine to keep as the local-dev seed; the WorkOS path simply doesn't use them.

What we do **not** touch: the `core/reviewrun` orchestrator, the Lens machinery, the docconv pipeline, the matters/documents/findings tables, the FE matter screens. The integration is identity-only.

---

## 7. Open questions worth flagging

- **Role naming.** Prototype uses `admin` and `member`. Schema has `memberships.role TEXT NOT NULL` with no CHECK. Decide whether to constrain it now (`CHECK role IN ('admin','member','owner')`) or leave open. Recommend constraining now; widening a CHECK is one-line, narrowing it later is a data-migration pain.


**Vineel: Leave open for now.**

- **`is_solo` flag.** Today defaulted to `TRUE`. When does it flip? Two sane triggers:

  - On `organization_membership.created` event, if the new active-membership count for the org is ≥ 2, flip `is_solo` to FALSE.
  - Or never auto-flip, leave it as a customer-tier decision driven by Stripe later.

   Recommend the first; it's a UX signal (show the dept picker), not a billing signal.


**Vineel: leave it default=true. That's the only flow we're going to build for the immediate future.**

- **Multi-org users.** The prototype already handles this in `auth.Callback` via `ListOrganizationMemberships`. The lazy-upsert path needs to upsert *all* memberships, not just the active one — otherwise an org-switch in the dashboard won't show up locally until the events drain catches up. (Solo signup ignores this — solos have exactly one membership by definition. Surfaces in the firm phase.)

**Vineel: We are only building solo right now, and leaving room for the bigger options later.**

---

`typora notes/claude-code-artifacts/workos-solomocky-integration-plan.md`

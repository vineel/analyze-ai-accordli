# Scaffolding + Mocky — Starter Build Plan

Three terms structure this plan. Use them precisely:

- **Scaffolding** — the permanent plumbing: auth (WorkOS), billing (Stripe), queue (River), database, file storage (Blob), LLM client + vendor failover, Reviewer's runtime (queue + Lens fanout + Prefix builder), observability, encryption posture, lifecycle (soft delete, export, hard delete), CI/CD, infra. Built once, kept forever.
- **Mocky** — the throwaway stub app sitting in the middle of the Scaffolding. A deliberately mocked-up product surface (signup, Matters list, upload, two stub Lenses, a basic detail page) whose only job is to exercise every Scaffolding subsystem end-to-end. Lives in `/web` and the HTTP edge of `/api`.
- **Analyze** — the real product app that will eventually replace Mocky once the product team finalizes the spec. Same Scaffolding underneath; different surface and a real Lens set.

The shape today, and after the eventual swap:

```
today:    [ Scaffolding ]  [ Mocky ]    [ Scaffolding ]
later:    [ Scaffolding ]  [ Analyze ]  [ Scaffolding ]
```

The guiding rule: **Mocky and its two stub Lenses are throwaway; the Scaffolding around them is permanent.** When Analyze replaces Mocky, the two stub Lenses get deleted, the real Lens set drops into Reviewer's runtime, the per-review-type aggregator replaces the starter summary call, and Mocky's UI is rewritten as Analyze's UI. Everything else — every Scaffolding subsystem — stays.

Reviewer is part of Scaffolding (its queue + fanout + Prefix builder + LLM client are permanent); the *Lens content* it runs is what evolves with the Mocky → Analyze swap.

Companion documents: `Reviewer-v2.md` (the runtime spec for Reviewer-the-subsystem), `accordli_platform_overview.md` (account model, plans), `stripe-implementation-guide.md`, `workos-implementation-guide.md`, `postgres-encryption-guide.md`, and `claude-code-artifacts/starter-lens-prompts.md` (the two stub Lens prompts Mocky will run).

---

## 1. Product surface (user POV)

A solo practitioner signs up via WorkOS AuthKit, lands on a "Matters" list, clicks **+** to create a Matter, uploads one .docx, and waits while the document is parsed and analyzed. The Matter detail page shows two per-Lens spinners that resolve into counts ("12 facts", "6 questions"); the user can expand each panel to see the JSONL-derived rows. A summary block (also LLM-generated, single call, not a Lens) sits above the panels. A Download Original button serves the .docx from Blob.

Once a ReviewRun has been initiated against a Matter, the Matter is locked — same invariant as Reviewer. No second upload, no second run on the same Matter.

That is the entire product surface. No team plans, no invites, no SSO, no admin tools, no reports, no editing of findings, no PDF export. All of those are deferred or covered separately.

---

## 2. What gets swapped, what survives

When Mocky → Analyze happens:

**Thrown away (Mocky-shaped):**

| Layer | Why it goes |
|---|---|
| Mocky's signup/Matters/upload/detail UI | Replaced by Analyze's real UI |
| Two stub Lenses (`entities_v1`, `open_questions_v1`) | Replaced by the real Lens set |
| Starter summary call | Replaced by Reviewer's per-review-type aggregator |

**Kept and extended (Scaffolding):**

| Layer | What happens |
|---|---|
| Reviewer runtime (queue + per-Lens job pattern + Prefix builder) | Stays; Prefix builder extended for new metadata + supplemental docs (same shape) |
| `findings` table (narrow stable shape + JSONB details) | Stays |
| `review_runs`, `lens_runs`, `matters`, `documents` tables | Stay |
| Vendor A→B failover | Stays |
| WorkOS, Stripe, Postmark, PostHog, Helicone integrations | Stay |
| Reserve / Commit / Rollback around a Run | Stays |
| Encryption posture, RLS, audit log | Stay |

If a piece of work belongs in the "thrown away" bucket, it should be the smallest possible piece that proves the surrounding Scaffolding works. If it belongs in "kept," it deserves real care now.

---

## 3. Stack at a glance

Locked decisions, applied to the starter:

| Concern | Choice |
|---|---|
| Cloud | Azure, East US 2; staging + prod subscriptions |
| API | Go (single binary, two roles via flag: `api`, `worker`) |
| Frontend | Vite + React + TypeScript |
| Database | Azure Postgres Flexible Server, Burstable B2s, CMK-encrypted |
| Queue | River (Postgres-backed) |
| File storage | Azure Blob, Hot tier, ZRS, versioning enabled |
| Document conversion | pandoc CLI in the worker container, Go post-processing |
| LLM (Vendor A) | Claude Sonnet 4.x via Azure AI Foundry, ZDR configured |
| LLM (Vendor B) | Anthropic API direct |
| LLM observability | Helicone (full bodies in dev/staging; **metadata-only in prod**) |
| Identity | WorkOS AuthKit (solo path only at starter) |
| Billing | Stripe direct (no Orb); Stripe Tax enabled |
| Email | Postmark |
| Product analytics + flags | PostHog Cloud (US region) |
| Secrets | Azure Key Vault |
| Source control / CI | GitHub + GitHub Actions |
| Hosting | Azure Container Apps (API, Worker, Admin containers) |
| Edge / rate limiting | Cloudflare in front of public origins |

Two things deserve a flag:

- **PostHog over a custom analytics layer.** Picked because it bundles product analytics, session replay (off by default for legal-data sensitivity), and feature flags in one SDK; that's three things the starter would otherwise need to wire separately. Tradeoff: another vendor with PII access. Mitigation: send only event names + user/org UUIDs to PostHog; never event properties carrying contract content. Same posture as Helicone-prod: metadata only.
- **No Orb.** Stripe is the canonical billing answer; the older "Orb in front of Stripe" line in `CLAUDE.md` is stale. The Stripe implementation guide is the spec we build to.

---

## 4. Repo layout (target)

```
/api               Go API + worker (single binary, two entrypoints)
  /cmd/api         HTTP server
  /cmd/worker      River worker
  /internal/...    domain code
/web               Vite + React + TS
/db
  /migrations      goose migrations
/prompts
  /lens            Go templates: entities_v1, open_questions_v1
  /summary         single non-Lens summary prompt
/infra
  /bicep           Azure infra-as-code (Flex, Blob, Key Vault, Container Apps)
  /workos          config snapshot per workos-implementation-guide.md
  /stripe          fixtures for staging product/price/meter setup
/notes             specs (existing)
/scripts           dev helpers (db reset, fixture data, etc.)
```

The `/api` and `/web` directories are the future production app; `/prompts` and `/db/migrations` are where the starter's domain artifacts accumulate.

---

## 5. Data model (starter)

Narrow stable shape; JSONB details where Lens-specific or Stripe-specific data lives. The intent is that adding a Lens later does not require an ALTER TABLE.

```
organizations
  id                  uuid pk
  workos_org_id       text unique not null
  stripe_customer_id  text unique
  name                text
  tier                text                -- 'solo' at starter
  is_solo             boolean default true
  billing_status      text                -- 'active' | 'past_due' | 'canceled'
  metadata            jsonb default '{}'
  created_at          timestamptz
  deleted_at          timestamptz         -- soft delete

departments
  id                  uuid pk
  organization_id     uuid fk
  name                text
  is_default          boolean default false

users
  id                  uuid pk
  workos_user_id      text unique not null
  email               text unique not null
  current_dept_id     uuid fk
  created_at          timestamptz
  deleted_at          timestamptz

memberships
  id                  uuid pk
  workos_membership_id text unique
  user_id             uuid fk
  organization_id     uuid fk
  department_id       uuid fk
  role                text
  status              text

matters
  id                  uuid pk
  organization_id     uuid fk
  department_id       uuid fk
  created_by_user_id  uuid fk
  title               text
  status              text                -- 'draft' | 'locked'
  locked_at           timestamptz
  created_at          timestamptz
  deleted_at          timestamptz

documents
  id                  uuid pk
  matter_id           uuid fk
  kind                text                -- 'original' | 'markdown'
  blob_url            text                -- canonical for 'original'
  content_md          text                -- canonical for 'markdown'
  filename            text
  size_bytes          bigint
  sha256              text
  created_at          timestamptz

review_runs
  id                  uuid pk
  matter_id           uuid fk
  organization_id     uuid fk
  status              text                -- pending | running | completed | partial | failed
  prefix              text                -- the assembled prompt body, sans Lens suffix
  prefix_token_count  int
  reservation_id      uuid                -- billing reserve
  vendor              text                -- 'A' | 'B'
  created_at          timestamptz
  completed_at        timestamptz

lens_runs
  id                  uuid pk
  review_run_id       uuid fk
  lens_key            text                -- 'entities_v1' | 'open_questions_v1'
  lens_template_sha   text                -- git SHA of the template
  status              text                -- pending | running | completed | failed
  retry_count         int
  vendor              text                -- 'A' | 'B', current
  finding_count       int                 -- populated on completion
  error_kind          text                -- nullable; 'vendor_error' | 'parse_error' | ...
  started_at          timestamptz
  completed_at        timestamptz

findings
  id                  uuid pk
  review_run_id       uuid fk
  lens_run_id         uuid fk
  lens_key            text                -- denormalized for indexing
  category            text                -- nullable; cross-Lens enum
  excerpt             text                -- nullable; ≤200 chars
  location_hint       text                -- nullable
  details             jsonb not null      -- everything Lens-specific
  created_at          timestamptz

usage_events            -- per stripe-implementation-guide.md
credit_ledger           -- per stripe-implementation-guide.md
reservations            -- per stripe-implementation-guide.md
review_billing_outcomes -- per stripe-implementation-guide.md
billing_periods         -- per stripe-implementation-guide.md
plans                   -- per stripe-implementation-guide.md
processed_webhooks      -- shared dedupe table for Stripe + WorkOS
audit_events            -- per workos-implementation-guide.md
```

Notes:

- `findings` carries `category` as the only stable enum; `kind` (Lens 1) and `severity` (Lens 2) live in `details` until product evidence justifies promoting one. This matches the agreed answer: keep the table stable through Lens churn.
- Postgres RLS on `matters`, `review_runs`, `lens_runs`, `findings`, `documents`, scoped by `organization_id`, as a defense-in-depth layer behind the API's auth middleware.
- All sensitive content (markdown, prefix, findings) is stored as ordinary types at v1 — protected by Flex CMK + TLS + access controls. Application-layer encryption is on the deferred roadmap; column types are chosen so a future migration to `bytea` is not destructive.

---

## 6. Subsystem-by-subsystem build

Each subsystem has a defined scope inside the starter and a clear deferral list.

### 6.1 Identity (WorkOS)

**In starter:** AuthKit hosted signup/login (email+password + Google OAuth), JWT-cookie session, `accordli_dept_id` JWT-template claim, webhook mirror into `users` / `organizations` / `memberships`, `audit_events` dual-write helper, signup creates default Org + default Dept + owner Membership inline.

**Deferred:** SSO/SAML, SCIM (Directory Sync), Admin Portal link generation, multi-Org users, "log out everywhere", invite flow, MFA-required policies, Audit Log SIEM streaming.

The signup flow at the starter is the solo path from `workos-implementation-guide.md` §5.6. No team plan UI. Tom's "we don't care about multi-Org-per-user right now" stays in force.

### 6.2 Billing (Stripe)

**In starter:** the `Solo Pro` plan only (the catalog can list more, only Pro is purchasable). One Stripe Product, one Price, one Meter (`arc_usage`), one monthly Credit Grant on signup, Reserve / Commit / Rollback wrapped around every ReviewRun, meter event on Commit, Customer Portal deep-link for cancel, Stripe Tax enabled, Smart Retries dunning configured.

**Deferred:** packs (one-off ARC purchases), refund flow, plan changes, team plans, enterprise net-30 invoicing, reconciliation cron, manual plan-change runbook UI.

The starter exercises Reserve/Commit/Rollback because Reviewer needs it. It does **not** exercise the second purchase path (packs) or any of the operations infrastructure (reconciliation), because those are stable by Phase 2 of the Stripe guide and not load-bearing for the swap to Reviewer.

### 6.3 Database (Azure Flex + River)

**In starter:** Flex Server (Burstable B2s) with CMK in Accordli's Key Vault, TLS 1.3 floor, `pgaudit` enabled, least-privilege roles (one for the API, one read-only for analytics, one for migrations), River schema installed, daily logical backups in addition to Flex's native backups.

**Deferred:** read replicas, point-in-time-restore drills automation, application-layer envelope encryption, per-Org DEKs, Managed HSM.

The CMK is configured from Day 1, not retrofitted, because the security questionnaire language in `postgres-encryption-guide.md` §"Security questionnaire language" is the customer-visible artifact and we want it true the moment we open to a beta user.

### 6.4 File storage (Azure Blob)

**In starter:** one storage account, Hot tier, ZRS, versioning enabled, soft-delete (14-day) enabled, lifecycle rule moves originals to Cool after 90 days. SAS URLs minted by the API for direct downloads (short TTL, scoped to one blob).

**Deferred:** customer-managed-key on Blob (Microsoft-managed key is fine for v1), private endpoint (public origin behind Cloudflare is fine for v1), per-Org container scoping.

### 6.5 Document conversion

**In starter:** `pandoc` binary baked into the worker container image. A `convert.docx_to_markdown` River job: reads the original from Blob, runs pandoc with the GFM target, post-processes in Go (strip empty headings, normalize whitespace, drop image references that didn't survive conversion), writes markdown to `documents.content_md`.

**Deferred:** PDF input, OCR, multi-document Matters, image extraction, table normalization beyond pandoc's defaults.

### 6.6 LLM (Anthropic Claude via Foundry, with failover)

**In starter:** every LLM call goes through a single Go `llm.Client` interface with two implementations (Foundry, Anthropic-direct). The Prefix is one Anthropic message-shaped block with `cache_control: ephemeral`. Lens jobs send `Prefix + Lens suffix` as two blocks; only the first is cached. Vendor A is the default; on a vendor error (per the Reviewer-v2 classification), the LensRun retries up to 2 on A, then switches to B. The Prefix step uses the same logic: 2 attempts on A, then full-run flip to B.

**Helicone:**
- **Dev / staging:** full prompt + response logging.
- **Prod:** metadata only — strip `messages` and response body before forwarding. Build the env-conditional behavior into the client wrapper; do not rely on a Helicone dashboard toggle.

The summary call (non-Lens) is the easiest place to prove the cache works — same Prefix as the Lens calls, the second call to Claude on a given Matter shows a cache hit. Verify in Helicone staging on Day 1 of the integration.

**Deferred:** model routing by Lens type, per-Lens model overrides, batch API, prompt-cache TTL extensions beyond ephemeral, fine-tuning, evals harness.

### 6.7 Reviewer-shaped fanout (the hot path)

The whole point of the starter. Implemented as:

```
Matter created
   │
   ▼
upload .docx → Blob → convert job (pandoc) → markdown stored
   │
   ▼
user clicks "Run Analysis"
   │
   ▼
Reserve(1 ARC) → review_run row inserted, Prefix built and stored
   │
   ▼
dispatch summary job + lens.entities_v1 job + lens.open_questions_v1 job
   ▼          ▼                              ▼
 (each: read prefix, send to Vendor A, buffer JSONL,
  parse, validate against Go struct, persist Findings,
  mark LensRun completed; on parse/vendor failure, follow
  the failover ladder; on terminal failure, mark failed.)
   │
   ▼
when last LensRun terminates:
  if completed_count >= 0.9 * total:
    Commit(reservation) → ledger -1, Stripe meter event
  else:
    Rollback(reservation) → free outcome row
   │
   ▼
UI sees per-Lens spinners resolve as each LensRun lands
```

Polling: the FE polls `GET /matters/:id/run` at 2s while any LensRun is `pending` or `running`. No websockets at the starter (deferred). Each spinner renders a count when its row hits `completed`.

The 90% threshold is silly with two Lenses (it means "both must complete or it's free") but the math is the right shape, and Reviewer-v2 inherits it unchanged. We keep the rule exactly as specified.

### 6.8 Email (Postmark)

**In starter:** transactional templates for welcome, password reset (proxied from WorkOS), payment failed, payment recovered. One Server (one stream: transactional). DKIM + return-path DNS configured under `mail.accordli.com`.

**Deferred:** marketing stream, broadcast/segmented sends, in-app inbox, push notifications, SMS.

### 6.9 Observability

Three layers, kept distinct:

- **App logs / metrics / traces.** Structured JSON logs from Go to Container Apps stdout → Log Analytics. OpenTelemetry SDK for traces; Azure Monitor as the backend at the starter. No Datadog.
- **LLM observability.** Helicone, env-conditional payloads (above).
- **Product analytics + feature flags.** PostHog Cloud. Events keyed on `org_id` and `user_id`; never carry document content as event properties. Feature flags evaluated server-side from the API for any flag that gates a billable behavior; client-side only for pure UI variations.

PostHog earns its place by giving us feature flags from Day 1 — we can ship "the second Lens" or "vendor B failover" behind a flag and test in staging with prod-shape traffic. If we end up wanting only flags, LaunchDarkly is the standard alternative; PostHog is the simpler SKU at this scale.

### 6.10 DevOps & deployment

**In starter:**
- Local dev: docker compose with Postgres, Azurite (Blob emulator), Stripe CLI in webhook-forward mode, WorkOS staging environment over the public internet.
- CI: GitHub Actions, three jobs (lint, test, build); push to `main` triggers a staging deploy via Container Apps revisions.
- Staging: full stack on Azure, traffic-routable to a new revision; Stripe staging account; WorkOS staging environment.
- Prod: same shape as staging in a separate subscription; manual promotion from a successful staging revision; Stripe + WorkOS production accounts.
- Bicep for the Azure surface (resource group, Flex, Key Vault, Storage, Container Apps Environment, three Container Apps).

**Deferred:** blue/green ish revision splits beyond the default 100/0, multi-region, autoscaling rules tuned beyond defaults, custom dashboards in Azure Monitor.

### 6.11 Customer support / lifecycle

**In starter:**
- **Soft delete.** `matters.deleted_at` and `users.deleted_at` and `organizations.deleted_at`. Queries default to `WHERE deleted_at IS NULL`. A 30-day sweep job hard-deletes soft-deleted rows and their Blob objects.
- **Data export.** A button in the (yet-to-exist) admin tool that writes a zip into Blob: JSON of every row scoped to the Org plus the original .docx files. Signed URL emailed to the requester.
- **CCPA-style hard delete.** A separate flow per `workos-implementation-guide.md` §5.11. Confirmation email, 24-hour window, transactional purge across Postgres, Blob, WorkOS Org and User.

**Deferred:** an actual admin UI (a couple of authenticated CLI commands are enough at the starter), in-product impersonation, customer-initiated export self-serve UI.

---

## 7. Build phases

Each phase is a coherent merge target. Earlier phases are not undone by later ones; later phases build through real seams.

### Phase 0 — Foundations (week 1–2)

Goal: an empty Go API and Vite frontend deploy to staging Azure with the supporting infra in place.

- Repo layout per §4. Goose migrations runnable from a Make target.
- Bicep for the staging environment: resource group, Flex Server **with CMK**, Key Vault, Storage Account, Container Apps Environment, three Container Apps (api / worker / admin), Application Insights, Log Analytics workspace.
- Two Stripe accounts (staging, prod). Two WorkOS environments. Two PostHog projects.
- Cloudflare in front of `staging.accordli.com`.
- Stripe CLI wired to local dev for webhook forwarding.
- `/health` endpoint, structured-log baseline, OpenTelemetry SDK initialized.
- GitHub Actions: lint, test, build, deploy-to-staging on `main`.
- TLS 1.3 floor enforced on Postgres.

Exit criteria: `curl https://staging.accordli.com/health` returns 200; the Vite app shows a hello world; River jobs can be enqueued and processed in staging.

### Phase 1 — Identity (week 2–3)

- WorkOS AuthKit on the frontend, `/auth/callback` exchange on the API.
- JWT-cookie session, JWKS-cached middleware, request context populated.
- `users` / `organizations` / `departments` / `memberships` / `audit_events` migrations.
- Webhook handler (`/webhooks/workos`) with signature verification, dedupe on `event.id`, mirror logic per the guide §5.5.
- Solo signup flow per the guide §5.6 — but **without the Stripe step**; signup at this phase produces an Org with no `stripe_customer_id`. (Stripe is wired in Phase 4; we don't want to block the auth track on billing.)
- `/me` endpoint returning the resolved Org/Dept/User from the validated JWT.
- "Hello, {{ .OrgName }}" landing page after login.

Exit criteria: a brand-new user can sign up, land on the dashboard, log out, log back in. WorkOS dashboard and Postgres rows agree.

### Phase 2 — Matters and files (week 3–4)

- `matters`, `documents` migrations. RLS policies installed.
- `POST /matters` (creates a draft Matter). `GET /matters` (lists for the Org/Dept). `GET /matters/:id`.
- Frontend Matters list page, Create-Matter button.
- File upload: `POST /matters/:id/upload` → API streams to Blob with a server-side SAS, writes a `documents` row of kind `original`.
- `convert.docx_to_markdown` River job: pandoc + Go cleanup, writes a `documents` row of kind `markdown`.
- Matter detail page renders the markdown. Download Original button uses a short-TTL SAS URL.

Exit criteria: a logged-in user uploads `sample.docx`, the page shows the converted markdown within ~10s, and the original downloads cleanly.

### Phase 3 — Single-Lens hot path (week 4–5)

- `review_runs`, `lens_runs`, `findings` migrations.
- Prefix builder: assembles system prompt + Matter metadata + markdown into the shared block; computes token count; stores on `review_runs.prefix`.
- `llm.Client` interface; Vendor A (Azure Foundry) implementation only.
- `lens.entities_v1` River job: reads prefix from the Run row, calls Claude with `Prefix + Lens suffix`, buffers JSONL, parses, validates against the `EntitiesFinding` Go struct, persists Findings all-or-nothing.
- `POST /matters/:id/run` — kicks off a ReviewRun (no billing gate yet).
- Matter detail page polls the Run; shows one spinner; resolves to "N facts" with an expandable table.

Exit criteria: a Run completes end-to-end on a real .docx; Findings persist; the UI shows the count and the rows.

### Phase 4 — Two Lenses + cache verified (week 5–6)

- Add `lens.open_questions_v1` job and `OpenQuestionsFinding` Go struct.
- Dispatch both Lenses + the summary in parallel from `POST /matters/:id/run`.
- Verify the prompt cache hit in Helicone (staging only — prod doesn't see bodies). The second-and-third calls on the same Run should report a cache hit on the prefix block.
- Per-Lens spinners on the Matter detail page; each resolves independently.
- PostHog wired: events for `matter_created`, `run_started`, `run_completed`, `lens_completed`, with `org_id` and `lens_key` properties only.

Exit criteria: a Run dispatches three calls in parallel, all three return, the UI shows two spinners resolving at different times, Helicone shows cache hits on the second and third, PostHog shows the events.

### Phase 5 — Vendor failover (week 6)

- Vendor B (Anthropic direct) implementation behind the same `llm.Client` interface.
- Failover ladder per Reviewer-v2 §"Failure & Retry":
  - Lens-level: 2 retries on A, switch to B, 2 retries on B, then `failed`.
  - Prefix-level: same shape, but the whole Run flips vendor.
- Vendor-classifying error helper (which errors trigger an immediate switch vs which retry on the same vendor).
- Synthetic failure injection in staging (a query-string flag on `/run` that forces an A failure on the next call) to exercise the path.

Exit criteria: with synthetic A failures injected, the Run still completes via B; `lens_runs.vendor` reflects the switch.

### Phase 6 — Billing (week 7–8)

- `plans`, `billing_periods`, `usage_events`, `credit_ledger`, `reservations`, `review_billing_outcomes`, `processed_webhooks` migrations.
- Stripe staging fixtures: `prod_solo_pro`, the licensed Price, the metered Price, the `arc_usage` Meter.
- Embedded Checkout in the signup flow: `/signup?plan=solo_pro` → AuthKit → backend creates Org/Dept/User → opens Stripe Checkout → completes Subscription.
- `checkout.session.completed` handler creates the first monthly Credit Grant + ledger row.
- Renewal-grant handler on `invoice.created` with `billing_reason = subscription_cycle`.
- Reserve / Commit / Rollback wrapper around `POST /matters/:id/run`. Reserve at job dispatch, Commit on ≥90% lens success, Rollback otherwise. Stripe meter event on Commit only.
- Customer Portal deep-link in /account.
- Smart Retries configured in the dashboard; `invoice.payment_failed` handler sets `billing_status = past_due` and triggers a Postmark email.

Exit criteria: a brand-new user signs up, pays $200, gets 10 ARCs, runs a Matter, and `credit_ledger` shows -1 and the Stripe meter shows the event. A second sign-up where the test card declines lands the org in `past_due` and the user gets a retry email.

### Phase 7 — Lifecycle, support, email (week 9)

- Soft delete on Matters, Users, Organizations. Sweep job (30-day).
- Data export: signed-URL zip.
- Hard-delete flow per `workos-implementation-guide.md` §5.11.
- Postmark templates wired to: welcome, payment failed, payment recovered, export ready, account deletion confirmation.

Exit criteria: an Org can be soft-deleted, exported, and hard-deleted. Each path emits the right email and writes the right `audit_events`.

### Phase 8 — Productionization (week 10+)

- Reconciliation cron (Stripe ↔ ledger) per `stripe-implementation-guide.md` §16.
- Monitoring dashboards + alerts per the same guide §20.
- Runbooks for: CMK rotation, vendor outage, Stripe webhook gap, WorkOS webhook gap, Helicone outage.
- Production cutover: prod Stripe + WorkOS + PostHog environments, prod DNS, prod Cloudflare config, prod Key Vault, first prod deploy.
- Beta-customer onboarding checklist.

Exit criteria: prod is up, has one real customer, and pages on real anomalies.

---

## 8. Explicit deferrals (so we stop relitigating these)

Not in the starter, by design:

- Team plans, invites, multi-user UI, shared ARC pools.
- SSO, SCIM, Admin Portal flow.
- Enterprise net-30 invoicing.
- ARC pack purchases (one-off Credit Grants).
- Refund flow (subscription or pack).
- Plan changes (upgrade / downgrade flows).
- Application-layer envelope encryption, per-Org DEKs, HYOK, Managed HSM.
- Multi-document Matters, PDF input, OCR.
- Real-time updates (websockets / SSE); polling is fine at the starter.
- Reports, memos, PDF generation.
- Custom domains for tenants.
- Session-replay analytics; PII-laden PostHog properties.
- Self-serve admin UI; CLI is enough.

Each of these has a home in either Reviewer's roadmap or one of the implementation guides.

---

## 9. Open questions / decisions to confirm

These are non-blocking for Phase 0 but should be resolved before the phase that needs them.

1. **Bicep vs Terraform for IaC.** The starter assumes Bicep because the Azure-native path is shortest. Terraform is the long-term standard if we expect multi-cloud; we don't. Confirm Bicep is fine.
2. **Container Apps vs AKS.** Container Apps is the simpler SKU at this scale and the starter assumes it. AKS becomes worth it when we want sidecar patterns, custom networking, or sophisticated autoscaling. Confirm Container Apps for Phase 0.
3. **Where the summary call lives.** Starter spec puts it as a third parallel call alongside the two Lenses, persisted on the `review_runs` row (not as Findings). Confirm — alternative is to make it a Finding with `category = 'summary'`, which is simpler but conflates Review-level and Lens-level outputs. Recommendation: keep it on the Run row.
4. **Phase-3 billing gate.** The starter spec gates billing on Phase 6, meaning Phases 3–5 produce free Runs. Acceptable for staging-only; the alternative is to wire a no-op Reserve/Commit shim earlier. Recommendation: defer the shim to Phase 6 as planned.
5. **Reviewer-v2's 90% rule with two Lenses.** "≥90% of Lenses succeeded" rounds to "both Lenses succeeded" with two Lenses. Mathematically correct but trivially strict. Confirm we keep the rule as-is (preferred — same rule everywhere) or relax for the starter.
6. **PostHog event schema lock.** Once events ship to PostHog, renaming them is painful. Recommendation: write the event catalog (a table of `event_name → required props`) into this doc before Phase 4 ships.

---

## 10. What "done" means for the starter

When all eight phases are merged — the full Scaffolding stack with Mocky in the middle — we should be able to:

- Sign up a new solo user with a real card on prod.
- Have them upload a .docx, run two parallel Lenses against it with a verified prompt cache hit on call 2 and 3, see Findings persisted with stable + JSONB shape.
- Charge them for the Run (or not, if it failed).
- Survive a simulated Vendor A outage end-to-end.
- Soft-delete their account, export their data, hard-delete on request.
- Show a customer security reviewer the encryption posture and have it match the language in `postgres-encryption-guide.md`.

At that point the Scaffolding is real; the only thing missing is Analyze. When the product team finalizes the spec, we delete Mocky — its UI, its two stub Lenses, the starter summary call — and drop Analyze (real UI, real Lens set, per-review-type aggregator) into the same Scaffolding. The Scaffolding doesn't move.

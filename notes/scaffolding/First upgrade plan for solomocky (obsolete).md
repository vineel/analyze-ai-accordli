# SoloMocky → Mocky — Incremental Build Plan (local-first)

The original `starter-app.md` was written as a from-scratch plan that started with Azure. We took a different path: we built SoloMocky end-to-end first, with every Scaffolding seam stubbed, and proved the runtime shape works. This doc replaces §7 of `starter-app.md`.

**Local-first stance.** We're deferring Azure deliberately. Every Scaffolding component that *can* run locally will be built and exercised on developer laptops first. Azure cutover (`Phase 8`) is the last phase, not the first. And: even after Phase 8, the local dev loop remains a durable, supported path — `make dev` works forever.

The intended audience is Vineel, Tom, and Claude Code working through this together over the next ~8 weeks.

---

## Where we are

SoloMocky is a single Go binary + Vite/React frontend that executes every *step* of the Reviewer pipeline (docx → markdown → prefix → summary + two Lenses → commit/rollback → finalize) on localhost. The work is async with respect to the HTTP request — `POST /api/matters` returns immediately and the pipeline runs separately — but it's not durable. The `GoroutineDispatcher` "queue" is `go func() { handler() }`: no buffer, no persistence, no retries. A `kill -9` mid-run leaves `review_runs` rows stuck in `running` forever. Phase 3 (River) is what makes the pipeline survive a restart.

| Seam                   | SoloMocky impl                                               | Phase that adds the prod impl                    |
| ---------------------- | ------------------------------------------------------------ | ------------------------------------------------ |
| `auth.Provider`        | `HardcodedProvider` (returns Mocky user)                     | 1                                                |
| `queue.Dispatcher`     | `GoroutineDispatcher` (`go func()` per dispatch — no buffer, no persistence, no retries) | 3                                                |
| `storage.Blob`         | `LocalFSBlob` (`file://` URLs)                               | 8                                                |
| `billing.Reserver`     | `NoopReserver`                                               | 5                                                |
| `llm.Client`           | `AnthropicDirect` (no failover, no Helicone)                 | 4 (failover + Helicone), 8 (Foundry as Vendor A) |
| `observability.Logger` | `StdoutLogger`                                               | 6 (OTel + PostHog), 8 (App Insights backend)     |
| Hosting                | `make dev` on localhost                                      | 8 (Container Apps)                               |
| Multi-tenant safety    | API-layer `org_id` predicate; no RLS                         | 2                                                |
| Identity model         | One seeded Org/Dept/User                                     | 1                                                |
| Lifecycle              | None                                                         | 7                                                |

---

# Phasing principles

1. **Local dev is permanent.** Every seam keeps both impls. Local picks one (`LocalFSBlob`, `AnthropicDirect`-as-A, etc.), prod picks the other. We never delete the local impl.
2. **One seam per phase.** A phase that swaps two seams at once doubles "what broke." 
3. **Each phase ends demoable.** Same end-to-end Matter run working, just routed through more real components.
4. **Order by what unblocks others.** Identity is a hard prerequisite for RLS, billing, lifecycle. Failover is a prerequisite for Stripe Reserve/Commit semantics. Observability lands late because instrumenting nothing-real isn't useful.
5. **No half-finished swaps.** When a phase ends, the new impl is wired and the seam interface is unchanged — future phases shouldn't be hunting for which impl is live.

---

## The plan

Eight phases. Phase 0 is dev tooling; Phases 1–7 are application work running entirely on localhost; Phase 8 is the Azure cutover at the end.

### Phase 0 — Local dev tooling

**Goal:** every developer can receive real WorkOS / Stripe / Helicone webhooks at a stable local URL, run the full stack with `make dev`, and have a reproducible Postgres.

**In scope:**

- **docker-compose for Postgres 17.** Replace `brew install postgresql@16` instructions with a docker-compose service pinned to PG 17. Reproducible across machines, matches prod, doesn't fight whatever else lives on the laptop. Version rationale: PG 17 is GA on Azure Flexible Server; PG 18 released Sept 2025 but isn't on Flex yet as of this writing. If 18 reaches Flex GA before Phase 8, we bump local and prod together — major-version moves are cheap until we have customer data.
- **Per-developer Cloudflare Tunnel.** Each of us gets a stable subdomain (`vineel.dev.vinworkbench.com`, `tom.dev.vinworkbench.com`) pointing to localhost via `cloudflared`. The `vinworkbench.com` zone lives in Vineel's Cloudflare account — keeps dev plumbing off `accordli.com`. One DNS record per developer, persistent across tunnel restarts, no expiring URLs. `make tunnel` brings it up. Document the one-time setup in `README.md`.
- **WorkOS local dev: no CLI equivalent of `stripe listen`.** WorkOS doesn't ship a webhook-forwarding CLI. The pattern is: register each developer's tunnel URL as an additional webhook endpoint in the shared WorkOS staging environment dashboard. WorkOS fan-outs deliveries to all configured endpoints, so every dev gets the same event stream concurrently. AuthKit redirect URIs are likewise configured per-developer in the same dashboard.
- **Stripe CLI** as the alternative for Stripe specifically (`stripe listen --forward-to localhost:8080/webhooks/stripe`). Some devs may prefer it over the tunnel for Stripe; both should work.
- **External accounts** in `.env.example`: one shared WorkOS staging environment (each dev's tunnel URL added as an extra webhook endpoint), Helicone dev key, PostHog dev project, Postmark sandbox, Stripe staging key. Document which are shared and which are per-developer.
- **`make dev` upgrades:** auto-loads `.env`, prints the API health URL, surfaces the tunnel URL if running.

**Out of scope:**

- Any application-code change.
- CI/CD (Phase 8).

**Exit criterion:** clone, `make dev`, `make tunnel` — receive a webhook fired from Stripe Dashboard or WorkOS test panel at the tunnel URL, and watch it hit a placeholder route in the API.

**Estimated weight:** small. ~1–2 days, mostly per-developer credential and DNS setup.

---

### Phase 1 — Real identity (WorkOS)

**Goal:** real signup / login / logout. Mocky user goes away.

**In scope:**

- WorkOS AuthKit on the FE; `/auth/callback` exchange on the API.
- JWT-cookie session, JWKS-cached middleware, `*auth.Identity` populated from the validated JWT.
- `auth.Provider` swap: add `WorkOSProvider` as a sibling impl. `HardcodedProvider` stays — it's the local-test-fixture impl, used by integration tests and Claude Code sessions that don't want to round-trip through WorkOS. App in dev/prod mode uses `WorkOSProvider`.
- Migrations to fill out `users` / `organizations` / `departments` / `memberships` / `audit_events` / `processed_webhooks` per `starter-app.md` §5. `solomocky.Seed` deleted from the run-at-startup path; lives on as a `--seed-test-org` admin command for tests.
- Webhook handler `/webhooks/workos` with signature verification + dedupe on `event.id`. Mirror logic per `workos-implementation-guide.md`. Webhooks come in via the dev tunnel.
- Solo signup creates Org + default Dept + owner Membership inline, **without** the Stripe step (Phase 5 adds it).
- Postmark sandbox wired enough to proxy WorkOS password-reset and welcome.

**Out of scope:**

- SSO/SAML, SCIM, Admin Portal, multi-Org users, MFA-required policies, invite flow, marketing email.

**Exit criterion:** a brand-new email signs up via AuthKit, lands on Matters, runs a Matter, logs out, logs back in. WorkOS dashboard rows and Postgres rows agree. Webhook replay is a no-op (dedupe works). Integration tests still pass using `HardcodedProvider`.

**Estimated weight:** large. ~1.5 weeks.

---

### Phase 2 — RLS + DB role separation

**Goal:** a request authenticated as Org X cannot read Org Y data, even if API code forgets to filter.

**In scope:**

- RLS policies on `matters`, `documents`, `review_runs`, `lens_runs`, `findings`, scoped by `organization_id`. `org_id` denormalized onto `lens_runs` and `findings` (already in schema) so policies stay single-table.
- Three DB roles in local Postgres: `accordli_app` (RLS-enforced — what api+worker connect as), `accordli_admin` (`BYPASSRLS` — for the future admin container and for goose-fix scenarios), `accordli_migrate` (`BYPASSRLS` — what goose runs as).
- API connects as `accordli_app` and runs `SET LOCAL app.current_org = $1` per request transaction.
- Integration test: forge a request body with another Org's matter UUID; expect 404, not a leak.
- pgaudit enabled in the docker-compose Postgres image.

**Out of scope:**

- Pattern B (admin "scope-as-Org") per `starter-app.md` §5. Keep BYPASSRLS for now.
- Per-Org DEKs / application-layer encryption (deferred indefinitely).

**Exit criterion:** the cross-Org integration test passes. A Postgres session as `accordli_app` with `app.current_org` unset returns zero rows from `matters`.

**Estimated weight:** medium. ~3–5 days. Single topic.

---

### Phase 3 — Persistent queue (River)

**Goal:** ReviewRuns survive an api/worker restart.

**In scope:**

- River schema migration in the same Postgres DB (separate schema).
- `queue.Dispatcher` swap: add `RiverDispatcher`. `GoroutineDispatcher` stays as the test-fixture impl (used by integration tests that don't want a real River).

- Job-level retries (default 2) with backoff. Failure classification.
- A small `/admin/queue` view (gated by an admin role check we'll fully build in Phase 7) for in-flight and dead-letter jobs.
- **Fanout commit at the end of the phase:** today's orchestrator runs Lenses sequentially in one goroutine. Per Reviewer-v2, the durable shape is one River job per LensRun. Resist doing the fanout mid-phase — keep the handler as-is, just persisted, then convert to fanout as the closing commit with a test that two LensRuns run concurrently.
- Integration test: kill the worker mid-Run, restart, the Run resumes from its current LensRun and completes.

**Out of scope:**

- Cross-instance scheduling, priority queues, scheduled (delayed) jobs, batch fanout APIs. Available in River; don't need them yet.

**Exit criterion:** `kill -9` mid-Run, restart, Run completes. `lens_runs.retry_count` reflects retries on transient LLM errors.

**Estimated weight:** medium. ~1 week.

---

### Phase 4 — LLM failover scaffold + Helicone

**Goal:** Vendor A → B failover works under synthetic outage; Helicone wired with the right env-conditional payload posture; prompt cache verified.

**Important:** real Vendor A is Anthropic-via-Foundry, which needs Azure. We're deferring that piece to Phase 8. **What we build now is the failover plumbing, exercised with two `AnthropicDirect`-shaped vendor configs** (different keys / endpoints, both pointing at api.anthropic.com). When Phase 8 lands, swapping in a `FoundryClient` is a config + one new impl file, not a rewrite of the failover logic.

**In scope:**

- Two `llm.Client` configs ("A" and "B"), both `AnthropicDirect` for now, distinct credentials.
- Failover ladder per Reviewer-v2 §"Failure & Retry": Lens-level 2-on-A → switch → 2-on-B → fail; Prefix-level same shape, whole Run flips vendor.
- Vendor-classifying error helper: which Anthropic error codes flip vs retry.
- Helicone wrapper around both vendor configs, **env-conditional in code** (full bodies in dev/staging-mode; metadata-only in prod-mode). Prod-mode strip is unit-tested independently of Helicone config.
- Synthetic failure injection in dev: `?force_vendor_a_fail=1` query-string flag on `POST /matters/:id/run` that fails the next A call. Not wired in prod-mode.
- Verify the `cache_control: ephemeral` block hits cache on calls 2 and 3 of a Run via Helicone dev dashboard.

**Out of scope:**

- Foundry-as-Vendor-A — Phase 8.
- Per-Lens model overrides, batch API, prompt-cache TTL extensions, evals harness.

**Watch out for:** the env-conditional payload strip is a security control, not perf. The unit test is mandatory; don't merge without it.

**Exit criterion:** with synthetic A failures injected, a two-Lens Run still completes via B; `lens_runs.vendor` reflects the switch; Helicone dev shows cache hits on calls 2 and 3; prod-mode unit test asserts no `messages` body forwarding.

**Estimated weight:** medium-large. ~1 week.

---

### Phase 5 — Billing (Stripe)

**Goal:** real card, real plan, real meter, Reserve/Commit/Rollback wired into ReviewRuns. All running locally with Stripe staging.

**In scope:**

- Migrations: `plans`, `billing_periods`, `usage_events`, `credit_ledger`, `reservations`, `review_billing_outcomes`. (`processed_webhooks` already exists from Phase 1.)
- Stripe staging fixtures: `prod_solo_pro`, the licensed Price, the metered Price, the `arc_usage` Meter. Captured as a `stripe fixtures` JSON in `infra/stripe/`.
- Embedded Checkout in the signup flow per `stripe-implementation-guide.md` §5–7.
- `checkout.session.completed` webhook → first monthly Credit Grant + ledger row.
- `invoice.created` (`billing_reason = subscription_cycle`) → renewal grant.
- `billing.Reserver` swap: add `StripeReserver`. `NoopReserver` stays as the test-fixture impl. App default in dev/prod-mode is `StripeReserver`. Reserve at job dispatch, Commit on ≥90% lens success, Rollback otherwise. Stripe meter event on Commit only.
- Customer Portal deep-link in `/account`.
- `invoice.payment_failed` → `billing_status = past_due` + Postmark "payment failed" email. Smart Retries configured in Stripe staging.
- Stripe Tax enabled on the staging account.
- `/webhooks/stripe` with sig verification + dedupe (reuses `processed_webhooks`). Webhooks come in via Stripe CLI or the dev tunnel.

**Out of scope:**

- Packs (one-off ARC purchases), refunds, plan changes, team plans, net-30, reconciliation cron (Phase 8), enterprise invoicing.

**Exit criterion:** a fresh signup with a real Stripe test card → 10 ARCs granted → Run a Matter → `credit_ledger` shows -1 + Stripe meter shows the event. A second sign-up with a declining card → `past_due` + retry email.

**Estimated weight:** large. ~2 weeks. The Stripe guide is the spec; stick to it.

---

### Phase 6 — Observability + product analytics

**Goal:** we can answer "what happened to this user's Run?" and "how many Runs per day?" without grepping logs.

**In scope:**

- OpenTelemetry SDK in the Go binary. **Local exporter:** OTLP to a local OTel collector (docker-compose service) that writes to stdout. Pick a hosted backend (Honeycomb / Grafana Cloud / etc.) only when stdout-grepping actively hurts; not worth the account-setup tax until then. The Azure-native exporter (App Insights) is added in Phase 8 alongside the prod exporter config.
- Spans on every River job and every HTTP handler. Trace ID surfaced in JSON logs.
- PostHog Cloud (US region), one dev project: events for `matter_created`, `run_started`, `run_completed`, `lens_completed`, with `org_id` and `lens_key` properties only. Never doc content. Same metadata-only posture as Helicone-prod.
- Server-side feature flags from PostHog for any flag gating a billable behavior. Client-side only for pure UI variations.
- **Event catalog locked before merge** in `notes/scaffolding/event-catalog.md`. Renaming PostHog events later is painful.

**Out of scope:**

- Session replay (off for legal-data sensitivity).
- App Insights backend wiring (Phase 8).
- Custom dashboards.

**Exit criterion:** a full Run end-to-end is visible as one trace in the local backend (HTTP → River dispatch → orchestrator → 3 LLM calls). PostHog dashboard shows the four event types. A dev-only feature flag gates a no-op behavior change end-to-end.

**Estimated weight:** medium. ~1 week.

---

### Phase 7 — Lifecycle (soft delete, export, hard delete)

**Goal:** we can credibly answer SOC 2 / CCPA / "delete my data" questions.

**In scope:**

- Soft delete: `deleted_at` on `matters`, `users`, `organizations`. Queries default to `WHERE deleted_at IS NULL`. 30-day sweep job (River cron) hard-deletes soft-deleted rows and their blob objects. (Locally, blobs are filesystem files; Phase 8 generalizes the sweep to AzureBlob via the `storage.Blob` interface — no logic change needed.)
- Data export: a CLI command (`go run ./cmd/admin export --org <id>`) writes a zip into the storage seam with JSON of every row scoped to the Org plus the original .docx files. Signed URL emailed via Postmark.
- CCPA-style hard delete per `workos-implementation-guide.md` §5.11. Confirmation email, 24-hour window, transactional purge across Postgres, blob storage, and WorkOS Org + User.
- Postmark templates: welcome (already from Phase 1), payment-failed/recovered (from Phase 5), export-ready, account-deletion-confirmation.
- Admin role: a simple `is_admin` claim on the WorkOS user metadata, gating the CLI commands and `/admin/queue` view.

**Out of scope:**

- Self-serve admin UI.
- In-product impersonation.

**Exit criterion:** an Org can be soft-deleted, exported, and hard-deleted via the CLI. Each path emits the right email and writes the right `audit_events` rows. After hard delete, the Org's blobs are gone and the WorkOS Org is deleted.

**Estimated weight:** medium. ~1 week.

---

### Phase 8 — Azure cutover

**Goal:** real customers, real money, real Azure. The application code stops moving here — everything in this phase is environment work.

**In scope:**

- **Bicep** for both subscriptions: staging RG + prod RG. Each has Flex Postgres (CMK from Day 1), Storage Account (ZRS, versioning, soft-delete), Container Apps Env, Key Vault, Application Insights, Log Analytics. Production-grade from the moment it exists; we don't "harden later."

**Vineel: What is Bicep? What is RG? What is ZRS?**

- **Storage seam:** add `AzureBlob` impl behind the existing `storage.Blob` interface. SAS minting for downloads. `LocalFSBlob` stays for local dev. Selected by env (e.g., `BLOB_BACKEND=local|azure`).
- **LLM seam:** add `FoundryClient` impl behind `llm.Client`. Slot it in as Vendor A by config. `AnthropicDirect`-as-A path remains as the local-dev default. Verify ZDR is configured.
- **Observability seam:** add the App Insights OTel exporter alongside the local one; selected by env.
- **Splitting api ↔ worker:** single binary, two roles via `--role=api|worker`. Two Container Apps from the same image. Worker has no public ingress. API scales on HTTP RPS; worker scales on River queue depth. Local dev keeps the combined binary.
- **GitHub Actions:** lint, test, build, deploy-on-merge to staging. Manual promotion gate to prod.
- **Cloudflare** in front of `staging.accordli.com` and `app.accordli.com`. WAF on, rate limits on the public `/auth/*` and `/webhooks/*` paths.
- **Prod accounts:** prod Stripe + WorkOS + PostHog. Postmark prod stream. Helicone prod project (with the metadata-only posture from Phase 4 already enforced in code).
- **Reconciliation cron** (Stripe ↔ ledger) per `stripe-implementation-guide.md` §16.
- **Monitoring + alerts:** webhook failure rate, queue depth, vendor failover events, `past_due` count.
- **Runbooks:** CMK rotation, vendor outage, Stripe webhook gap, WorkOS webhook gap, Postgres failover, Helicone outage.
- **First prod customer.**

**Out of scope:**

- Multi-region.
- AKS.
- Custom domain per tenant.
- The admin Container App. Defer until ops pain motivates it.

**Exit criterion:** prod is up, has one paying customer, pages on real anomalies. Local dev still works unchanged with `make dev`. SoloMocky → Mocky transition complete.

**Estimated weight:** large. 2–3 weeks, mostly Bicep + Azure resource provisioning + drill rehearsals.

---

## Order rationale

A few of these phases could be reordered. The recommended order optimizes for unblocking and cognitive cohesion. Tradeoffs we considered:

- **Phase 0 first.** Without webhook tunneling, Phases 1 and 5 stall. Cheap to land; do it once.
- **1 (auth) before 2 (RLS).** RLS without multiple Orgs is untestable. 2 before 5 means the most sensitive tables (`credit_ledger`, `reservations`) are born under RLS, not retrofitted.
- **3 (River) before 4 (LLM failover).** Failover involves retries with backoff; doing that with a persistent queue is cleaner than a goroutine pool. Not load-bearing — could swap order.
- **4 (failover) before 5 (Stripe).** Stripe's Reserve/Commit/Rollback wraps the LLM calls. We want failover stable before billing depends on its outcome.
- **6 (observability) late.** PostHog and OTel are mostly useless without real flows. Earlier instrumentation gets ripped out as code shapes change.
- **7 (lifecycle) before 8 (Azure).** "Can you delete my data" is a question prod customers ask. Don't ship prod without it.
- **8 (Azure) last.** With everything else proven locally, Azure becomes a configuration + deployment exercise, not a discovery exercise.

Reorders we'd entertain: **3 ↔ 4** if Tom and Vineel feel one is more urgent than the other.

---

## Things this plan deliberately does not change

- The shape of the Reviewer pipeline. Orchestrator stays sequential within a Run until Phase 3's fan-out commit; the contract with `core/lens` is stable.
- The data model. New tables and columns get added across phases; existing columns don't get renamed.
- **No migrations until first prod customer.** Until Phase 8 we don't run `goose` or accumulate `db/migrations/000N` files. Schema lives in a single `db/schema.sql`; any change is `drop database → create database → apply schema.sql → seed`. `make reset` is the only path. Migrations come back at Phase 8 when there's prod data to preserve. (Per-phase language below saying "migrations to add X" is shorthand for "edit `schema.sql` to add X" — Phases 1, 2, 3, 5 still need a one-pass reconciliation against this rule; tracked separately.)
- The seam interfaces. `auth.Provider`, `queue.Dispatcher`, `storage.Blob`, `billing.Reserver`, `llm.Client`, `observability.Logger` keep their current signatures. If a swap motivates a signature change, that's a flag — talk it through before merging.
- Local impls. `HardcodedProvider`, `GoroutineDispatcher`, `LocalFSBlob`, `NoopReserver`, `AnthropicDirect`, `StdoutLogger` all stay forever as either dev-mode defaults or test fixtures.
- The 90% lens-success rule with two Lenses (silly but consistent with Reviewer-v2; don't optimize).
- The two stub Lenses (`entities_v1`, `open_questions_v1`). They remain throwaway until Mocky → Analyze.

---

## Open questions to resolve before kicking off

1. **Cloudflare Tunnel domain.** What hostname do we use? `*.dev.accordli.com` is the natural choice; needs the Cloudflare account to own the zone. Confirm.

**Vineel: Let's use *.dev.vinworkbench.com. I have easy access to that, and I'd rather keep the messy stuff away from accordli.com.**

1. **WorkOS staging environments — per-developer or shared?** Per-developer is cleaner; shared is cheaper. Recommendation: per-developer until we have more than 4 of us.

**Vineel: One shared.**

1. **Local OTel backend.** Honeycomb vs Grafana Cloud vs stdout-only. Recommendation: stdout-only in Phase 6, pick a hosted backend when it actually hurts.

Vineel: Yes.

1. **PostHog event catalog owner.** Must be written before Phase 6 ships. Recommendation: Tom drafts, Vineel reviews.

**Vineel: Sure, let's wait until we know more about PostHog.**

1. **Where the summary call lives.** Currently on `review_runs.summary`, not as a Finding. `starter-app.md` §9.3 recommendation: keep on Run. Confirm before any phase that touches the summary path (none of 1–8 does in scope, but worth nailing).

**Vineel: Sure, a summary is not an issue.**

1. **Postgres major version pin.** Whatever we put in docker-compose at Phase 0 is what we'll provision in Azure at Phase 8. Confirm: 16.

**Vineel: Why 16? Isn't 18 current?**

These are non-blocking for Phase 0 start. Resolve them in parallel.

---

./notes/claude-code-artifacts/solomocky-to-mocky-plan.md
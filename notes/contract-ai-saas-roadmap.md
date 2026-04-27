# Contract Analysis SaaS — Build Roadmap

## Requirements Summary

**Product.** AI agent for contract analysis. Users upload contracts, get analysis and printable/exportable reports. Adjacent functionality TBD pending prototype feedback (~3 weeks).

**Audience.** B2B. Primary users are lawyers — high security expectations, sophisticated buyers, procurement-driven sales motion likely.

**Team & stack.**
- 2 engineers (you + one collaborator using Claude Code)
- Backend: Go
- Frontend: TypeScript + React
- Cloud: **Azure** (chosen based on team experience, customer profile likely Microsoft-shop, and Claude availability via Microsoft Foundry)

**Business model.**
- Seat-based subscription with tiered plans (e.g. Starter / Pro / Enterprise)
- Each plan includes N contract analyses per period (per seat or per org)
- Overage handled via pay-as-you-go and/or pre-purchased credit packs
- Tiers differentiate on quota, features, support, and possibly compliance posture

**Non-functional requirements.**
- Security-forward at launch — defensible story for legal buyers on day one
- SOC 2 readiness as a continuous goal; Type I within ~9 months, Type II thereafter
- Auth, observability, and metering must be capable of enforcing the billing model accurately
- Data handling defensible for confidential legal documents (no training on customer data, retention controls, audit trails)

**Horizon.** 6–12 month roadmap, not just the prototype window.

---

## Part 1 — Problems to Solve and Off-the-Shelf Components

Organized roughly in the order they become load-bearing. "Buy" means use the SaaS; "OSS" means self-host an open-source option if you prefer.

### Identity, Auth, and Tenancy

| Problem | Recommendation | Notes |
|---|---|---|
| Login, sessions, password reset | **WorkOS** (AuthKit) | B2B-first. SSO/SAML, SCIM, audit logs, organizations all native. Worth its price for legal customers. |
| Alternative for self-serve | Clerk | Better React UX, weaker enterprise story |
| OSS option | Ory or Zitadel | More work, more control |
| User directory sync | WorkOS SCIM | Required for enterprise deals; drives seat reconciliation |
| Org/workspace concept | Use WorkOS Organizations | Maps 1:1 to billing entity |

### Billing and Metering

| Problem | Recommendation | Notes |
|---|---|---|
| Subscription billing + invoicing | **Stripe Billing** to start; **Orb** when you outgrow it | Orb handles seat+included+overage+credits natively |
| Sales tax | **Stripe Tax** | Do not roll your own |
| Customer self-service (portal, payment method updates, invoices) | Stripe Customer Portal | Free with Stripe Billing |
| Usage metering (long-term) | **Orb** or Metronome | Purpose-built for your model shape |
| Dunning / failed payment recovery | Stripe Smart Retries + your own emails | |

### AI Infrastructure

| Problem | Recommendation | Notes |
|---|---|---|
| LLM API calls + fallback + cost tracking | **Helicone** or Portkey | Don't call providers directly from app code |
| LLM provider routing | **Azure Foundry** for Claude (recommended) or direct Anthropic API | Foundry gives you Claude under your existing Azure billing/MACC, Entra ID auth on model endpoints, Azure Monitor for usage, and zero-data-retention commitments. Stack a thin gateway (Helicone) on top for prompt-level observability and caching. |
| LLM observability (prompts/responses/latency) | **Langfuse** (OSS, self-host) or Helicone | Critical for "the analysis was wrong" debugging |
| Eval harness | **Promptfoo** (OSS) or Braintrust | Build a 20-contract regression set early |
| Document parsing — `.docx` | OpenXML parsing in Go (your existing knowledge) | |
| Document parsing — PDF (native + scanned) | **LlamaParse**, Reducto, or AWS Textract | Don't burn weeks on PDF extraction |
| File storage | **Cloudflare R2** or **AWS S3** | R2 has no egress fees; S3 has the longer compliance pedigree |
| Vector search (if/when needed) | pgvector (in your existing Postgres) | Avoid adding a new DB unless forced |

### Application Plumbing

| Problem | Recommendation | Notes |
|---|---|---|
| Background jobs | **River** | Postgres-backed, idiomatic Go |
| Transactional email | **Resend** or Postmark | Resend has React Email templates |
| Marketing email / lifecycle | Loops or Customer.io | Defer until post-launch |
| Real-time progress for long jobs | Server-Sent Events from Go | Native, no extra infra |
| Rate limiting | Cloudflare (edge) + Redis token bucket (app) | |
| Search (in-product) | Postgres FTS to start; Typesense if needed | |
| PDF/report generation | Gotenberg (OSS, self-host) or DocRaptor | |

### Observability and Operations

| Problem | Recommendation | Notes |
|---|---|---|
| Error tracking | **Sentry** | Day one. Both Go and React SDKs. |
| Logs / metrics / traces | **Grafana Cloud**, BetterStack, or Axiom | Instrument with OpenTelemetry in Go from day one |
| Uptime monitoring | BetterStack or Checkly | Synthetic checks on critical paths |
| Status page | **Instatus** or BetterStack | Lawyers notice when AI degrades |
| Product analytics + feature flags + session replay | **PostHog** | One tool, generous free tier, self-hostable. Configure replay masking carefully for app pages. |

### Customer-Facing Surfaces

| Problem | Recommendation | Notes |
|---|---|---|
| Support inbox / ticketing | **Plain** or Crisp early; Intercom later | Email alias is fine for month one |
| In-app messaging / NPS | PostHog surveys to start | |
| Documentation site | **Mintlify** (API-heavy) or Docusaurus | |
| Marketing site | Next.js on Vercel or Astro | Keep it separate from the app |
| Changelog | A `/changelog` route or Headway | |

### Security, Compliance, Devops

| Problem | Recommendation | Notes |
|---|---|---|
| Cloud | **Azure** (primary recommendation) | Strong fit for B2B legal customers (Microsoft-shop friendly), Claude available via Foundry, MACC-eligible billing. AWS and GCP are also defensible; Azure picked here based on team experience and customer profile. Avoid Heroku/Render for app tier handling contracts. |
| Secrets management | **Azure Key Vault** | Native to Azure, integrates with Entra ID for access control. Doppler is a fine alternative if you want a cloud-agnostic tool. No `.env` files in prod, ever. |
| MDM (laptop management) | **Kandji** (Mac) or **Microsoft Intune** | Kandji is best-in-class for Mac fleets; Intune is the natural choice if you're already in the Microsoft ecosystem and want unified Entra ID management. Required for SOC 2 either way. |
| SSO for internal tools | **Microsoft Entra ID** or Google Workspace | If you're on Azure, Entra ID is the path of least resistance and familiar to enterprise legal customers. Pay the SSO tax on every SaaS tool. |
| Compliance automation | **Vanta** or Drata | Engage ~month 3–4 |
| External audit | A-LIGN, Prescient Assurance, Johanson, Insight Assurance | Type I ~month 9 |
| Annual pen test | **Cobalt** or HackerOne | $8–15k; required by enterprise customers |
| WAF / DDoS / bot protection | Cloudflare | |
| DAST / dependency scanning | GitHub Advanced Security or Snyk | |

### Legal and Policy

| Problem | Recommendation | Notes |
|---|---|---|
| ToS / Privacy / Cookie / DPA templates | Termly or Iubenda for drafts | Have a lawyer review before any B2B contract |
| Subprocessor list | Static page on marketing site | Lawyers will read it |
| AI-specific clauses | Custom; reference industry templates | "No training on customer data" is the headline sentence |

---

## Part 2 — Integrations and Glue You'll Build

These are the components that don't come off the shelf. They tie the bought pieces together and encode your product's specific logic.

### Tenancy and Auth Glue

1. **Organization model.** WorkOS Organization → your `organizations` table → billing customer in Stripe/Orb. Webhook-driven sync between all three.
2. **Membership and roles.** Owner / Admin / Member at minimum. Role enforcement in API middleware.
3. **Tenant isolation enforcement.** App-level `org_id` filtering on every query, plus Postgres Row-Level Security as a defense-in-depth net. Test with a "cross-tenant access" suite that runs on every PR.
4. **Session / device management UI.** Active sessions, device list, logout-everywhere.
5. **Invite and onboarding flows.** Email invite, accept-with-SSO, first-time-user setup.

### Usage, Metering, and Entitlements

1. **`usage_events` table.** Append-only, immutable. Columns: `id`, `org_id`, `user_id`, `kind`, `quantity`, `created_at`, `billing_period_id`, `idempotency_key`, `resource_id` (e.g. contract_id), `metadata`.
2. **`billing_periods` table.** Per-org, with plan snapshot so historical usage stays correct after plan changes.
3. **`credit_ledger` table.** For purchased credit packs and adjustments. Append-only with `delta`, `reason`, `expires_at`.
4. **Entitlement service in Go.** Pure function over the event log + ledger + plan: "given org X, kind Y, time T, what's the available balance?"
5. **Metering middleware.** Two-phase: `Reserve(org, kind, qty) → reservation_id`, then `Commit(reservation_id)` on job success or `Rollback(reservation_id)` on failure. Reservations have a TTL.
6. **Soft and hard limit handlers.** Warning banners at 80%, hard 402 at 100% with upgrade CTA, configurable spend caps for admins.
7. **Period rollover job.** River cron that closes periods, generates invoices via Stripe/Orb, and starts new periods.
8. **Stripe/Orb webhook handlers.** Idempotent. `subscription.updated`, `invoice.paid`, `invoice.payment_failed`, etc.

### AI Pipeline

1. **Contract intake pipeline.** Upload → virus scan → parse (docx/pdf) → normalize → store → enqueue analysis job.
2. **Analysis orchestrator.** River jobs that call Helicone, stream progress via SSE, write results, and commit usage.
3. **Prompt versioning.** Prompts stored as DB rows or files in repo with version IDs; every analysis records the prompt version it used. Critical for explaining historical results and for evals.
4. **Eval pipeline on CI.** Promptfoo or Braintrust runs against your gold set on every prompt change.
5. **Per-org cost attribution.** Helicone tags every request with `org_id`; daily roll-up into a `cost_per_org` view. Surface gross margin per customer.
6. **Output storage and report generation.** Structured analysis stored as JSON; rendered to HTML/PDF on demand via Gotenberg or similar.
7. **Cross-tenant prompt leakage tests.** Automated test that no system prompt or vector context bleeds across orgs.

### Audit Log and Admin

1. **`audit_events` table.** Org-scoped. Every meaningful action: login, upload, analyze, view, export, share, delete, settings change.
2. **Audit log API + UI.** Filterable by user, action, date range. Exportable to CSV. Lawyers love this; it's also SOC 2 evidence.
3. **Internal admin tool.** Start as `/admin` route gated by email allowlist + MFA. Org list, user list, impersonation (with audit trail), refund/credit issuance, feature flag toggles. Move to Retool only when justified.
4. **Customer-facing usage dashboard.** Current period usage vs entitlement, historical trend, per-seat breakdown, predicted overage.

### Data Lifecycle

1. **Data export.** "Download all my data" button: contracts, analyses, audit log, metadata as a ZIP.
2. **Account/workspace deletion.** Soft-delete + scheduled hard purge (e.g. 30 days). Documented procedure. Cascade through all subsystems including S3 and Helicone logs.
3. **Configurable retention.** Per-org retention policy (e.g. delete contracts older than N days). Background job enforces.
4. **Backup + restore.** Automated DB backups, *tested* restore procedure. Document the test.

### Operational Surfaces

1. **Health checks and synthetic monitors.** Feeding both your APM and your status page.
2. **Feature flags wiring.** PostHog flags consulted in Go and React; documented flag lifecycle (create → ramp → remove).
3. **Per-org rate limits.** Both per-user (anti-abuse) and per-org (cost protection) at the app layer.
4. **Anomaly detection.** Alerts when an org's usage rate exceeds N× their baseline.
5. **Incident response runbook.** Pager rotation (even just two of you), playbooks for common incidents, postmortem template.

---

## Part 3 — SOC 2 Readiness, Billing Correctness, and Launch-Quality Security

Two parallel tracks: compliance posture and billing-system correctness. Both are about generating evidence and operating consistently.

### SOC 2 — Foundational Controls (set up early, run continuously)

**Cloud and infrastructure**
- [ ] Single Azure tenant; prod/staging in separate subscriptions under a management group
- [ ] All workloads in private VNets/subnets; no public DBs; bastion or Azure Bastion for shell access
- [ ] Encryption at rest (Azure Key Vault / customer-managed keys where appropriate) and in transit (TLS 1.2+) on every data store, queue, blob container
- [ ] Azure Activity Log + diagnostic settings shipped to a central Log Analytics workspace (immutable retention)
- [ ] NSG flow logs enabled
- [ ] Key Vault in use; no secrets in repos, images, or env files
- [ ] IaC for everything (Terraform or Bicep); manual changes alarmed
- [ ] **Account-safety hardening from day one** (lessons from prior AWS compromise): MFA enforced on all admin accounts, no long-lived root credentials, billing/spend alerts wired to email + SMS, hard spending limit on dev/test subscriptions, Conditional Access policies restricting admin actions, Microsoft Defender for Cloud enabled at standard tier, anomalous-cost detection turned on. The $$$-runaway-via-compromised-account scenario is the same on every cloud; the defense is the same too.

**Code and access**
- [ ] GitHub branch protection + required reviews + required status checks
- [ ] Signed commits required on protected branches
- [ ] Dependabot + secret scanning + CodeQL enabled
- [ ] Production access via SSO + MFA only; least privilege; break-glass documented
- [ ] Quarterly access reviews (compliance platform automates the workflow)

**Workforce**
- [ ] MDM enforced on all employee laptops (disk encryption, screen lock, OS updates, malware protection)
- [ ] All SaaS tools behind SSO
- [ ] Background checks documented for new hires
- [ ] Onboarding and offboarding checklists with revocation SLAs
- [ ] Annual security awareness training (Vanta/Drata bundle this)

**Policies (templates from compliance platform; sign and follow)**
- [ ] Information Security Policy
- [ ] Acceptable Use Policy
- [ ] Access Control Policy
- [ ] Change Management Policy
- [ ] Incident Response Plan
- [ ] Business Continuity / Disaster Recovery Plan
- [ ] Vendor Management Policy
- [ ] Risk Assessment (annual)
- [ ] Data Classification and Handling Policy
- [ ] Encryption Policy

**Vendors and data**
- [ ] Subprocessor inventory (publicly listed)
- [ ] DPA in place with every subprocessor handling customer data
- [ ] Anthropic / OpenAI / any LLM provider on zero-retention / no-training enterprise tier
- [ ] AWS BAA / DPA signed
- [ ] Vendor risk reviews documented (compliance platform tracks)

**Operational evidence**
- [ ] Centralized audit logging across cloud, GitHub, Workspace, app
- [ ] Documented backup and tested restore (with date of last test)
- [ ] Pen test scheduled annually; remediation tracked
- [ ] Vulnerability management process (scan cadence, severity SLAs)
- [ ] Change management (PR reviews + deploy logs serve as evidence)
- [ ] Incident log (even if empty)

**Audit logistics**
- [ ] Engage compliance platform around month 3–4
- [ ] 3+ months of operational evidence accumulated before audit window
- [ ] Type I audit ~month 6–9 (point-in-time)
- [ ] Type II audit window 3–12 months following (continuous operation)

**Public-facing security artifacts**
- [ ] `/security` or `/trust` page describing posture
- [ ] Security overview PDF (one page) for sales-cycle questionnaires
- [ ] Subprocessor list page
- [ ] Privacy policy with explicit AI data-handling language
- [ ] Standard DPA available on request

### Billing Correctness

**Data model**
- [ ] `usage_events` is append-only, with an idempotency key, and is the source of truth
- [ ] `billing_periods` snapshot the plan at period start
- [ ] `credit_ledger` is append-only with expirations
- [ ] No counters that get mutated; everything derived from event sums

**Enforcement**
- [ ] Two-phase reserve/commit/rollback wrapping every billable operation
- [ ] Reservation TTL prevents orphaned holds
- [ ] Hard-limit returns clear 402 with upgrade path
- [ ] Per-user and per-org rate limits prevent abuse and runaway cost

**Stripe/Orb integration**
- [ ] All webhooks idempotent (use webhook event ID as dedupe key)
- [ ] Subscription state mirrored locally; provider is authoritative for invoices, you are authoritative for usage
- [ ] Plan change handler (proration of seats and quota)
- [ ] Seat count synced from WorkOS SCIM events
- [ ] Failed payment → grace period → suspension flow defined and tested
- [ ] Refund and credit-issuance flow in admin tool

**Customer experience**
- [ ] Usage dashboard with real numbers (not "estimated")
- [ ] Soft alerts at 80%, hard at 100%, with upgrade CTAs
- [ ] Spend caps configurable by org admins
- [ ] Self-serve credit pack purchase
- [ ] Receipt and invoice download
- [ ] Per-seat usage breakdown for admins

**Operational**
- [ ] Monthly reconciliation: sum of `usage_events` matches what was billed
- [ ] Per-org gross margin dashboard (revenue vs LLM cost vs infra cost)
- [ ] Anomaly alerts on cost-per-org outliers
- [ ] Documented procedure for billing disputes and adjustments

### Day-One Security Story (what to tell legal buyers at launch)

A buyer-ready security narrative needs to credibly assert:

1. **Where their data lives.** Single cloud, named region, encrypted at rest and in transit.
2. **Who can see it.** Tenant isolation, least-privilege internal access, audit logged.
3. **What happens to it.** Retention configurable, hard-delete on request, exportable on demand.
4. **What the AI does with it.** No training on customer data. Zero-retention provider tier. Optional no-prompt-logging mode.
5. **How you'd know if something went wrong.** Logging, monitoring, incident response, breach notification SLA.
6. **What outsiders have verified.** SOC 2 in progress (then Type I, then Type II), annual pen test, vulnerability management.
7. **What you'll sign.** Standard DPA, willing to negotiate MSAs, subprocessor list maintained.

If the `/security` page can answer all seven without marketing fluff, you'll clear most legal-buyer due diligence on the first pass.

---

## Suggested Phasing (6–12 month view)

**Phase 0 — Prototype window (weeks 0–3).**
Sentry, basic WorkOS, Azure foundation (single tenant, prod/staging subscriptions, MFA on all admin accounts, billing alerts, spend limits on non-prod), Key Vault, MDM on laptops, Claude via Azure Foundry with zero-retention configured, `usage_events` table logging from day one (no enforcement), `audit_events` table, one-page security overview on the website. Helicone in front of Foundry + a 20-contract eval set during prototyping.

**Phase 1 — Paid launch (months 1–3).**
Stripe Billing live (seats + metered overage), customer usage dashboard, in-app limits enforcement, transactional email, status page, support inbox, public security/trust page, DPA template, subprocessor list, basic admin tool, data export and deletion flows.

**Phase 2 — SOC 2 readiness (months 3–6).**
Engage Vanta/Drata, write/sign all policies, formalize access reviews and onboarding/offboarding, run first pen test, complete vendor risk reviews, document BCP/DR with a tested restore.

**Phase 3 — Enterprise-ready (months 6–9).**
SSO/SCIM productized, audit log UI shipped to customers, role granularity expanded, configurable retention per workspace, spend caps, Type I audit. Migrate billing to Orb if model complexity justifies.

**Phase 4 — Scale and trust (months 9–12).**
Type II audit window in progress, security questionnaires automated, dedicated infra option for top-tier customers, compliance posture extended (HIPAA, ISO 27001) only if customer demand justifies the cost.

---

## Closing Note

The most expensive mistakes on a roadmap like this aren't the missing features — they're the load-bearing decisions made implicitly. The ones to make explicitly, early, even if you don't build against them yet:

- Tenancy model (shared DB with RLS vs. DB-per-tenant)
- Usage as event log, not as mutated counter
- LLM provider abstraction behind a gateway, not direct calls
- Prompt versioning from the first commit
- Audit log as a product feature, not just an internal artifact
- "No training on customer data" as a contractual and technical commitment

Those six choices, made well in the prototype window, will save months later.

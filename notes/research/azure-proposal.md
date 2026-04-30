# Minimal-Cost Production System on Azure (Starter Scale)

A concrete proposal for Accordli's first production deployment, given three load-bearing decisions:

- **Cloud:** Azure (single tenant, prod + staging subscriptions)
- **Auth/Identity:** WorkOS (AuthKit + Organizations, SSO/SCIM available)
- **Billing/Metering:** Orb (subscriptions, included usage, overage, credit packs)

The goal is the cheapest stack that is still credibly *production* — not a free-tier toy, but not enterprise-grade redundancy either. It must be defensible to a legal buyer on day one and able to grow into SOC 2 Type I in months 6–9 without re-platforming.

---

## 1. Scale Assumptions

These drive every cost number below. If reality is materially different, the bill changes.

| Assumption | Value |
|---|---|
| Paying organizations | 10–30 |
| Active users | 30–100 |
| Agreement Review Credits / month (total) | 400–800 |
| Avg contract size | ~30K input tokens, ~10K output tokens through Claude Sonnet 4.6 |
| Storage of contracts + outputs | < 50 GB total |
| Egress | < 200 GB / month |
| Engineers needing prod access | 2 |
| Region | One — `East US 2` (cheapest tier 1 region, paired with `Central US` for backups) |
| Availability target | 99.5% (single-region, single-AZ for app, ZRS for storage) |

We are explicitly *not* paying for multi-region failover, hot standbys, or 99.99% uptime at this stage. Those become Phase 3+ concerns.

---

## 2. Architecture (One Picture in Words)

```
   Cloudflare (DNS, WAF, CDN, free tier)
          │
          ▼
   Azure Front Door  — skipped at starter scale; Cloudflare fronts directly
          │
          ▼
   ┌──────────────────────────────────────────────────────────┐
   │   Azure Container Apps Environment (single env)          │
   │                                                          │
   │   ┌────────────────────┐    ┌──────────────────────┐     │
   │   │  api (Go)          │    │  worker (Go + River) │     │
   │   │  HTTP + SSE        │    │  background jobs     │     │
   │   └─────────┬──────────┘    └──────────┬───────────┘     │
   │             │                          │                 │
   └─────────────┼──────────────────────────┼─────────────────┘
                 │                          │
                 ▼                          ▼
   ┌──────────────────────────┐   ┌──────────────────────────┐
   │ Azure Postgres Flexible  │   │  Azure Blob Storage      │
   │  Server (Burstable B2s)  │   │  (Hot tier, ZRS)         │
   │  + pgvector              │   │  contracts + reports     │
   └──────────────────────────┘   └──────────────────────────┘

   Frontend (React/TS):  Azure Static Web Apps (Standard)
   Secrets:              Azure Key Vault
   LLM:                  Azure AI Foundry → Claude Sonnet 4.6
                         (production: direct call from worker;
                          Helicone proxy used in dev/staging only)
   LLM failover:         Direct Anthropic API (Vendor B per Reviewer-v2)
   Auth:                 WorkOS AuthKit (hosted)
   Billing:              Orb (hosted) ← Stripe (payment processor)
   Email:                Resend
   Errors:               Sentry
   Logs/metrics:         Grafana Cloud (free tier) via OTel
   Uptime:               BetterStack (free tier)
   Product analytics:    PostHog Cloud (free tier)
   Status page:          Instatus (free tier)
   CI/CD:                GitHub Actions
```

The hot path: a request hits Cloudflare → Container Apps API → enqueues a River job in Postgres → worker pulls the job, calls Foundry (Claude) directly → writes results to Postgres + Blob → SSEs progress back. Orb gets a metered usage event on commit; WorkOS owns identity; Sentry/Grafana see everything via OTel.

Production traffic does **not** route through Helicone. Helicone retains prompts and responses by default, and contract content cannot live in a third-party logging gateway without breaking the day-one trust narrative ("contract bytes live in encrypted Blob and in process memory only"). Helicone is used in dev and staging for prompt iteration and eval observability, against synthetic/test contracts only. If at some point we need production-grade prompt observability, the answer is self-hosted Helicone inside our Azure tenant with redaction configured, not the hosted gateway.

On vendor failover: the worker calls Foundry by default and falls back to the direct Anthropic API per the rules in `Reviewer-v2.md` (Failure & Retry). Both vendors are wired from day one; this is not a Phase 2 hardening item.

---

## 3. Document Processing Pipeline

This is the most fragile part of the data plane and the part a legal-buyer security questionnaire will probe hardest, so it gets its own section. Three concerns, one pipeline:

1. Get bytes from the user's browser into durable storage safely.
2. Verify the file is not malicious before any code reads it.
3. Convert `.docx` to a clean markdown representation that Claude can analyze, **without writing customer plaintext to local disk**.

### 3.1 Upload — server-mediated streaming

The Go API mediates the upload. The browser POSTs a multipart form to `/api/contracts`; the handler streams the multipart body directly into the Azure Blob SDK uploader, block by block. Bytes are never fully buffered and never written to local disk.

```
browser ──multipart POST──► api ──Azure Blob SDK stream──► uploads-quarantine
                              │
                              └─► reserve ARC, insert contracts row
                                  {status: "uploading"} → return 202 contract_id
```

Why proxied (not browser-direct via a SAS URL):

- Median `.docx` is < 5 MB, hard cap 50 MB. Resumability and edge-direct upload aren't solving a problem we have.
- Server-mediated upload composes cleanly with the two-phase metering pattern — `Reserve(org, "arc", 1)` happens in the same handler that creates the contract row, before bytes touch storage.
- One handler: one auth check, one tenant check, one DB transaction. CORS-on-storage-account misconfigurations are a classic foot-gun we don't need.

When median upload size grows past ~25 MB or replica count gets driven by upload concurrency, migrate to SAS direct-upload. The blob layout doesn't change, so it's a local refactor.

### 3.2 Virus scanning — Microsoft Defender for Storage (Malware Scanning)

Enabled on the storage account holding `uploads-quarantine`. Defender scans every blob on write asynchronously (typically < 30 s), tags the blob with a verdict, and emits an Event Grid event.

- Pricing: Defender for Storage plan ~$10/storage-account/mo + **$0.15/GB scanned**. At ~40 GB scanned/mo → ~$16/mo, folded into the §4.1 Defender line.
- Scanning happens **inside Azure** — no contract bytes leave the tenant. We do not use VirusTotal, Cloudmersive, or any third-party AV API that would upload contents.
- Hard policy boundary: the worker has no IAM read on `uploads-quarantine`. The pipeline literally cannot see an unscanned file.

Event-driven hand-off:

```
Defender scan complete event → router
  ├─ "No threats found"  → server-side copy to uploads-clean,
  │                         enqueue ParseDocxJob
  ├─ "Malicious"         → copy to uploads-infected (locked),
  │                         audit_event, notify org admin, no ARC charged
  └─ "Scan failed"       → retry once, then treat as infected
```

Defender catches *known-malicious* payloads. It does not catch zip bombs, XXE, malicious macros, or remote-template injection. Belt-and-braces controls:

- 50 MB upload size cap at the API.
- When unzipping: max decompressed size + max member count limits.
- Go stdlib `encoding/xml` does not process DTDs by default — keep it that way; reject any docx that smells of external entities.
- We are not currently re-distributing the original `.docx` to other org members. If that becomes a feature, strip `word/vbaProject.bin`, embedded OLE `.bin` parts, and external relationship targets in `settings.xml` before sharing.

### 3.3 Parse — docx2md + pandoc, in-memory in the worker

The parser is the existing `docx2md-go` library: an XML preprocess (`PreprocessDocx`) that fixes numbering, headings, and form-table unrolling, then `pandoc -f docx -t gfm`, then `NormalizeSectionHeadings` regex passes. Today it's `(srcPath, dstPath)` everywhere and `runPandoc` invokes pandoc with a path argument.

For production we add an **io-based core that never touches local disk**. The path-based API stays as a thin shim for the CLI and the corpus byte-equal tests.

```
ParseDocxJob (River, on the worker):
  in        := blob.Download(ctx, "uploads-clean/<org>/<id>.docx")   // []byte
  pre, st   := docx2md.PreprocessDocxBytes(in)                       // []byte → []byte
  md        := runPandocStdin(ctx, pre)                              // stdin → stdout
  md         = NormalizeSectionHeadings(md)
  blob.Upload(ctx, "derived/<org>/<id>.md", []byte(md))
  contracts.Update(id, status="parsed", parse_stats=st,
                   parse_version=docx2md.GitSHA,
                   pandoc_version="3.9.0.1")
  River enqueue: AnalyzeContractJob{contract_id}
```

Two new functions on `docx2md`:

- `PreprocessDocxBytes(in []byte) ([]byte, Stats, error)` — uses `zip.NewReader(bytes.NewReader(in), int64(len(in)))`. Zip's central directory lives at the end of the file, so the reader needs random access; the input is buffered fully in memory. That's RAM, not disk — and that's the distinction that matters.
- `runPandocStdin(ctx, in []byte) ([]byte, error)` — `cmd.Stdin = bytes.NewReader(in)`, stdout and stderr captured separately, `exec.CommandContext` with a 60–90 s hard timeout.

Memory peak per job is ~3× the docx size (input buffer + transformed buffer + pandoc's own zip decode). At a 50 MB cap that's ~150 MB per concurrent contract; a 1 GiB worker replica comfortably handles 5–6 concurrent jobs. Real contracts are < 5 MB and you'll never feel it.

What this buys us, in priority order:

1. **No customer plaintext on the writable layer, ever.** The trust page can assert "contract bytes live in encrypted Blob and in process memory only" without caveats.
2. **No `defer os.Remove` failure modes.** A worker SIGKILL mid-job leaves nothing to clean up.
3. **No `/tmp` pressure during retry storms.** A flapping job can't fill the writable layer.
4. **One less box in the data-flow diagram** for security questionnaires.

What it does *not* buy: speed. Pandoc reads the whole docx into memory either way; skipping a `/tmp` write is microseconds. The argument is operational and trust-narrative, not throughput.

### 3.4 Worker container image

The worker image must bundle **pandoc 3.9.0.1** — the version pinned by `docx2md-go`'s corpus byte-equal tests. A different pandoc silently produces different markdown. Multi-stage Dockerfile: install the pinned `.deb`, copy the binary into the Go runtime image. Adds ~150 MB; cold-start pull adds 1–2 s. Worker startup runs `pandoc --version` and refuses to boot on a mismatch.

The API image does **not** need pandoc. Build two images in CI; the API stays small, fast, and lower-attack-surface.

### 3.5 Reprocessing and ops

- **Idempotency.** `ParseDocxJob` keys on `contract_id`. Same input + same `parse_version` + same `pandoc_version` = byte-equal output. Tag the markdown blob with both SHAs.
- **Reprocess on upgrade.** When `docx2md` or pandoc bumps, enqueue `ParseDocxJob{contract_id, force=true}` for affected contracts. No ARC charged; new `parse_version` recorded.
- **Stats as telemetry.** `TitleToH1`, `Demoted`, `Promoted`, `LabelsInjected`, `LabelsSkipped` persist on the contract row (or a `processing_events` table). Surface in the admin tool — invaluable for "why did this analysis come out wrong?"
- **Pandoc patching cadence.** Pinned versions stabilize output; CVE bumps are a quarterly chore — bump version → re-run corpus goldens → re-derive existing `.md` files for affected contracts.
- **Failure handling.** Pandoc OOM or timeout → River retries → permanent failure → contract row `status="parse_failed"`, surface to user, two-phase metering rolls back the reservation, **no ARC charged**.

---

## 4. Component-by-Component Cost Breakdown

All prices are USD/month, list price, as of early 2026. Assumes you are *not* yet on a Microsoft Azure Consumption Commitment (MACC); MACC discounts kick in once you have an enterprise agreement, typically post-Series A.

### 4.1 Azure Compute and Data

| Resource | SKU | Monthly cost |
|---|---|---|
| **Container Apps** — `api` (1 replica min, 2 max, 0.5 vCPU / 1 GiB) | Consumption | ~$25 |
| **Container Apps** — `worker` (1 replica, 0.5 vCPU / 1 GiB, scale-to-zero off) | Consumption | ~$25 |
| **Postgres Flexible Server** — Burstable `B2s` (2 vCPU, 4 GiB, 64 GB SSD) | Burstable | ~$60 |
| **Postgres backups** — 7-day PITR, geo-redundant | Included | ~$10 |
| **Blob Storage** — Hot, ZRS, 50 GB + ~1M ops | Standard | ~$5 |
| **Static Web Apps** — Standard tier (custom domain, auth-enabled APIs, SLA) | Standard | $9 |
| **Key Vault** — Standard, low op count | Standard | ~$2 |
| **Log Analytics Workspace** — 5 GB/day cap, 30-day retention | Pay-go | ~$15 |
| **Microsoft Defender for Cloud + Defender for Storage Malware Scanning** (see §3.2) | Standard | ~$40 |
| **Bandwidth egress** — ~200 GB | First 100 GB free, rest $0.087/GB | ~$10 |
| **Reserved IPs / misc networking** | — | ~$5 |
| **Subtotal — Azure infra** | | **~$205 / mo** |

Notes:
- We do **not** use AKS. Container Apps is a managed Kubernetes-on-Azure that bills for actual compute consumed and scales to one replica idle. AKS has a control-plane fee plus VM costs that don't make sense at this scale.
- We do **not** use Azure Front Door at $35+/mo flat. Cloudflare's free plan covers WAF, DNS, CDN, basic DDoS, and works fine in front of Container Apps via CNAME.do
- We do **not** use Azure Cache for Redis. River is Postgres-backed; rate limits live in Postgres or in-memory at the app tier until volume forces a change.
- We do **not** use Azure Monitor's full APM suite. OTel from Go → Grafana Cloud is meaningfully cheaper at this scale and not vendor-lock.
- Defender for Cloud is *optional* at $0 (free tier covers CSPM only) but the $25 Standard tier gets server vulnerability scanning, anomalous-cost detection, and the "yes we have a CSPM" line on a security questionnaire — worth it.

### 4.2 LLM (Claude via Azure AI Foundry)

Single biggest variable cost. Azure Foundry list pricing for Claude Sonnet 4.6 matches Anthropic's public API pricing:

- Input (uncached): ~$3 / 1M tokens
- Cache write: ~$3.75 / 1M tokens (1.25× input)
- Cache read: ~$0.30 / 1M tokens (0.10× input)
- Output: ~$15 / 1M tokens

**Per-ARC cost model.** Reviewer-v2 splits one analysis into a shared **prefix** (system prompt + contract + supplemental docs + Matter metadata) consumed by ~12 parallel **Lenses**. The prefix is several hundred KB — call it ~75K–150K tokens — and is written to Anthropic's prompt cache on the first lens call, then read by every subsequent lens. Each lens emits a JSONL response of findings. The cost of one Full Review therefore decomposes into:

| Component | Per Review |
|---|---|
| Prefix cache write (once) | `prefix_tokens × $3.75/M` |
| Prefix cache reads (11 lenses, one is the writer) | `11 × prefix_tokens × $0.30/M` |
| Lens prompts (12 × ~1.5K uncached tokens) | `~$0.054` |
| Lens output (12 × output_tokens × $15/M) | `12 × output_tokens × $15/M` |

Ranging the two unknowns (prefix size, average lens output):

| Scenario | Prefix tokens | Output / lens | Per-ARC cost |
|---|---|---|---|
| Low (small contract, terse findings) | 50K | 3K | **~$0.95** |
| Mid (typical) | 100K | 5K | **~$1.66** |
| High (large contract, verbose findings or extended thinking) | 150K | 8K | **~$2.55** |

This is **5–13× higher than the prior `$0.20/ARC` estimate**, which assumed a flat 30K-input/10K-output single-call shape and did not model the fan-out of lenses against a large shared prefix. The mid case is the right number to plan margins around until we have measured production traffic.

The model is sensitive to two assumptions worth verifying early:

1. Foundry's `cache_control` semantics actually match the direct Anthropic API. If cache writes are charged differently (or caching isn't supported), the prefix-read line jumps from `$0.30/M` to `$3/M`, roughly doubling per-ARC cost. (Tracked in `notes/todo.md` item 14.)
2. Average output per lens. Findings are structured JSONL, but extended-thinking output can balloon. Track this from day one in Helicone (dev/staging) and the worker's own metrics (production).

**Eval / dev / regression runs:** budget another ~30% on top of production token spend for prompt iteration and regression evals on the gold set.

**Failover (Vendor B = direct Anthropic):** same per-token pricing, billed against an Anthropic account rather than Azure. Failover traffic is expected to be a small fraction of total volume; budget impact is rounding error in normal operation, but capacity must exist (account, credentials, billing alerts) from day one.

| Volume | Production ARC cost (mid) | + 30% dev/eval | Monthly LLM bill |
|---|---|---|---|
| 400 ARCs | $664 | $199 | **~$865** |
| 800 ARCs | $1,328 | $398 | **~$1,725** |
| 1,500 ARCs | $2,490 | $747 | **~$3,235** |

We will configure Foundry's zero-data-retention commitment from day one and quote it on the trust page. Foundry billing rolls into the Azure invoice, which simplifies MACC accounting later.

### 4.3 Auth — WorkOS

WorkOS pricing (as of 2026):

- **AuthKit:** free up to 1M MAU. Includes hosted login, password reset, MFA, magic links, social.
- **Enterprise SSO (SAML/OIDC):** $125 per connection per month, billed only for orgs that actually have SSO turned on.
- **Directory Sync (SCIM):** $125 per connection per month, similarly per-org.
- **Audit Logs API:** included free with AuthKit.
- **Organizations:** free, unlimited.

For starter scale, assume:
- 0–2 SSO connections live (most starter customers won't insist on SSO; the enterprise plan triggers it)
- 0–1 SCIM directories

**Monthly cost: $0–$375**, almost certainly **$0 for the first ~6 months**. Budget $250 starting around month 6 once the first enterprise deal lands.

### 4.4 Billing — Orb + Stripe

Orb is a metering and billing engine; it does not process payments. Stripe sits behind Orb as the payment processor.

**Orb pricing.** Public list pricing is roughly $720/mo for the entry production tier (the "Growth" / startup plan), or it's quoted as a percentage of billed revenue (often ~0.5–0.8% with a floor). Orb has historically had a free tier for very early-stage companies and a startup program; you should ask for it.

| Scenario | Orb monthly |
|---|---|
| Orb Startup Program (if eligible) | **$0** for ~12 months |
| Orb entry tier | ~$720 |
| Revenue-based (assume $30K MRR × 0.6%) | ~$180 |

**Realistic line item: $0 if accepted into the startup program, otherwise budget $500–$720.**

If Orb's price doesn't fit at starter scale, the roadmap's recommendation stands: **Stripe Billing for the first 6–12 months, migrate to Orb when complexity demands**. Stripe Billing handles seats + tiered + metered + credit packs adequately. The cost shape changes:

- **Stripe-only path:** Stripe takes 2.9% + 30¢ per charge (always), plus 0.5% on recurring billing line items, plus Stripe Tax at 0.5%. On $30K MRR that's roughly ~$1,200–$1,400 in payment-processing fees, but those scale with revenue and exist on either path.

The proposal here assumes we **do** use Orb because it was specified — and that we apply for the startup program. **Budget $0–$720/mo.**

**Stripe (payment processing only):** ~3% of GMV. Not a fixed line item, but real money — at $30K MRR, ~$900/mo. This is unavoidable on any path.

### 4.5 Observability and Operations

| Tool | Tier | Monthly |
|---|---|---|
| **Sentry** | Team (50K errors, 100K perf events) | $26 |
| **Grafana Cloud** | Free (10K series, 50 GB logs) | $0 |
| **BetterStack** uptime | Free (10 monitors, 3-min) | $0 |
| **PostHog** | Free (1M events, 5K replays) | $0 |
| **Instatus** status page | Free (5 components) | $0 |
| **Helicone** LLM gateway (dev/staging only) | Free up to 100K requests | $0 |
| **Subtotal — observability** | | **~$26 / mo** |

Free tiers will hold for 6+ months at this scale. Sentry's free tier (5K errors) is too thin for production; pay the $26.

**PostHog session replay** is a hard "off" on any page that renders Matter content (contract text, findings, reports). Recording a lawyer's screen while they read a confidential agreement is the kind of thing that ends a sales cycle. Replay is enabled on marketing, signup, account, and admin pages only. Treat this as a P0 design constraint, not a configuration footnote.

### 4.6 Email, Domains, Misc

| Item | Monthly |
|---|---|
| **Resend** transactional email (3K free, then $20 for 50K) | $0–$20 |
| **GitHub Team** (2 seats, required for branch protection + private repos at any seriousness) | $8 |
| **Domain registration** (amortized) | ~$2 |
| **Cloudflare** (free tier covers WAF/CDN/DNS/DDoS) | $0 |
| **Subtotal** | **~$10–30 / mo** |

GitHub Advanced Security (CodeQL on private repos) is $49/seat/mo and worth deferring until SOC 2 prep — for the prototype window, public-repo CodeQL or Snyk's free tier will do.

### 4.7 Security / Compliance Tools (Pre-SOC-2)

These are deferred but worth listing so they are not surprises in the Phase 2 budget.

| Tool | When | Cost when active |
|---|---|---|
| **Vanta or Drata** | Month 3–4 | ~$10–14K/year (~$900/mo) |
| **External SOC 2 audit** | Month 6–9 (Type I) | $15–25K, one-time |
| **Annual penetration test** | Month 9–12 | $8–15K, one-time |
| **Microsoft Intune** (MDM) | Month 1+ | $6/user/mo (~$12 for 2) |
| **1Password Business** (passwords / shared secrets UX) | Month 1 | $8/user/mo (~$16) |

Add **~$30/mo** for Intune + 1Password from day one. The rest land in Phase 2.

---

## 5. Total Cost — Starter Scale, Steady State

| Category | Low (400 ARCs) | Likely (800 ARCs) | High (1,500 ARCs) |
|---|---|---|---|
| Azure infra | $185 | $205 | $245 |
| LLM via Foundry (mid per-ARC, +30% dev/eval) | $865 | $1,725 | $3,235 |
| WorkOS | $0 | $0 | $375 |
| Orb | $0 | $0 | $720 |
| Observability | $26 | $26 | $26 |
| Email + GitHub + misc | $10 | $20 | $30 |
| Intune + 1Password | $30 | $30 | $30 |
| **Total (excl. payment processing)** | **~$1,115** | **~$2,005** | **~$4,660** |
| Stripe processing fees (~3% of MRR) | varies | varies | varies |

**Most likely realistic monthly bill in the first 6 months: ~$1,100–$1,300**, assuming Orb startup program acceptance, zero SSO connections live, and ~400 ARCs/month.

**Realistic month 12 bill** (one SSO customer live, Vanta engaged, Defender Standard, Orb on entry tier, ~800 ARCs/month): **~$3,000–$3,500/mo**.

For context, at the assumed scale:
- 20 orgs on a blended ~$500/mo plan = ~$10K MRR
- COGS at the likely line above is ~11–13% of revenue (driven almost entirely by LLM tokens)
- Still healthy gross margin, but materially tighter than the prior estimate. This is the line item that benefits most from prompt-engineering discipline (smaller prefixes, terser lens outputs) and from MACC-tier discounts on Foundry once we have an enterprise agreement.

---

## 6. What This Stack Covers

✅ **Production-grade application hosting** with auto-scaling, zero-downtime deploys, and managed certs.
✅ **Tenant-isolated data plane** with Postgres + RLS and Blob with per-org prefixes/SAS scoping.
✅ **Auth that legal customers will accept** — MFA, SSO-ready, SCIM-ready, audit logs, password reset.
✅ **Subscription + metered + credit-pack billing** matching the Pro/Gold/Team plans in the spec.
✅ **Claude under your Azure invoice**, with zero data retention contractually committed.
✅ **Two-vendor LLM failover** (Azure Foundry primary, direct Anthropic API fallback) per `Reviewer-v2.md`. Wired from day one; not deferred to Phase 2.
✅ **Day-one observability** — errors, traces, metrics, logs, uptime, and product analytics.
✅ **Security primitives needed for the Day-One Security Story** — encryption at rest/in transit, Key Vault, MFA, Defender, audit logs, data export, configurable retention.
✅ **A path to SOC 2 Type I in months 6–9** without re-architecting.
✅ **Single-region 99.5% availability**, which is a defensible launch posture.

## 7. What This Stack Does *Not* Cover

❌ **Multi-region failover.** A region-wide Azure outage takes Accordli down. Adding a hot standby would roughly double infra cost.
❌ **Multi-AZ Postgres.** Burstable tier is single-AZ. Upgrading to General Purpose with HA enabled is ~$300/mo on its own.
❌ **HIPAA / FedRAMP / ITAR / GovCloud.** None are in scope. If a customer demands HIPAA, the BAA path on Azure works but adds review work; FedRAMP is a multi-quarter project and a separate stack.
❌ **Private networking from customer to Accordli** (e.g., Azure Private Link). Available on request later for enterprise tier.
❌ **Customer-managed encryption keys (CMK / BYOK).** Azure-managed keys only at this stage; CMK is a Phase 3+ feature gated on Key Vault Premium.
❌ **24/7 on-call coverage.** Two-engineer rotation, best-effort, with documented escalation. Status page communicates real availability. Don't oversell this in contracts.
❌ **Dedicated tenant infrastructure.** Single shared cluster + DB. Dedicated stacks are an Enterprise-tier upsell, not a starter feature.
❌ **PDF OCR for scanned contracts** is *not* in this budget. LlamaParse / Reducto / Azure Document Intelligence each add $50–$300/mo at this scale; pick one when the first scanned-PDF customer shows up.
❌ **Marketing site, blog, docs site.** The marketing site lives on Vercel/Cloudflare Pages (free) outside this estimate.
❌ **eDiscovery, litigation hold, customer data subpoena tooling.** Manual procedures only at this stage. Lawyers will ask; the answer is "we will respond within X days via documented process."

---

## 8. Key Assumptions and Calls Made

These are the load-bearing decisions baked into the numbers above. If any of them is wrong, redo the math.

1. **Foundry Claude pricing tracks Anthropic public API pricing.** Verified true at time of writing; subject to MACC discounts later.
2. **Prompt caching delivers ~90% input-token savings** on cache reads (the prefix is read by 11 of 12 lenses). The first lens incurs a 1.25× cache-write premium on the prefix. The §4.2 cost model bakes this in. We will engineer for this from the first call. If Foundry's `cache_control` semantics turn out not to match the direct Anthropic API (`notes/todo.md` item 14), the per-ARC math roughly doubles.
3. **Orb startup program acceptance.** If denied, switch to Stripe Billing for 6–12 months; the table's "high" column is the worst case.
4. **Single region is acceptable for launch.** A buyer's security questionnaire might ask for DR posture; the answer is "documented BCP, geo-redundant backups in `Central US`, RTO 4h / RPO 1h, multi-region active-active on the Phase 4 roadmap."
5. **No first-year customers require HIPAA, FedRAMP, or in-region EU residency.** If an EU lawyer is the first sale, add a `West Europe` deployment; that's roughly +$200/mo.
6. **Two engineers manage everything.** No SRE, no dedicated security engineer. Vanta + Defender + GitHub Advanced Security carry the automation load.
7. **Container Apps over App Service.** Container Apps wins on scale-to-low cost and Kubernetes-shaped ergonomics; App Service is only cheaper if you commit to a year-long reserved instance, which doesn't fit a starter.
8. **Postgres Flexible Server Burstable B2s holds for ~6 months.** A 4 GiB DB with pgvector and ~1K analyses/mo is fine. Watch IOPS; the upgrade path to General Purpose is in-place.
9. **Helicone is the dev/staging LLM observability tool**, not a production gateway. Production traffic calls Foundry directly. Helicone vs. Portkey is a reversible decision; we picked Helicone for its free tier and OSS self-host story (relevant if we later want a production observability path inside our Azure tenant).
10. **No new analytics warehouse.** Postgres serves all reporting needs at this scale. A real warehouse (BigQuery / Snowflake / Fabric) is a Phase 3 question.

---

## 9. First-Month Build Order

Concrete, in priority order — what to stand up before customer one:

1. **Azure foundation:** management group, two subscriptions (`accordli-prod`, `accordli-staging`), MFA enforced on all admin accounts, billing alerts at $250/$500/$1000, hard spend cap on staging at $200/mo.
2. **Terraform repo** in GitHub: VNet, Container Apps Environment, Postgres Flexible Server, Blob Storage, Key Vault, Log Analytics Workspace.
3. **CI/CD pipeline** (GitHub Actions → ACR → Container Apps).
4. **WorkOS** project, AuthKit configured, Organizations as the customer model.
5. **Orb** sandbox, plans modeled (Pro / Gold / Small Team / Large Team / Extra Contract Pack), Stripe linked.
6. **Foundry** project, Claude model deployment, zero-retention commitment confirmed in writing. Worker calls Foundry directly in production; Helicone is wired into dev/staging only against synthetic test contracts. Verify `cache_control` semantics match the direct Anthropic API end-to-end (`notes/todo.md` item 14). Stand up the direct-Anthropic account for Vendor B failover at the same time.
7. **Sentry, Grafana Cloud OTel pipeline, BetterStack monitors, PostHog, Instatus** — all wired before the first customer sees the app.
8. **Application skeleton:** `usage_events`, `audit_events`, `billing_periods`, `credit_ledger` tables defined and emitting from day one (no enforcement yet).
9. **Trust page** at `/security` answering the seven-point Day-One Security Story from the roadmap.
10. **One end-to-end smoke test** that uploads a contract, runs a real Claude analysis (against a synthetic test contract), returns a report, and emits an Orb usage event — exercised every deploy. The dev/staging variant of this test also produces a Helicone log; the production variant does not.

That gets us to a defensible "ready to take a paying lawyer" state for under $500/mo, with every load-bearing piece in place to grow into SOC 2 readiness without re-platforming.

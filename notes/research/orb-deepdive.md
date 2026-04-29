# Orb Deep Dive — What's Worth Buying for Accordli's Use Case

A focused analysis of what Orb does that we *can't* easily build ourselves, given Accordli's specific pricing model and starter scale. The TL;DR is at the bottom; the body is the reasoning that gets us there.

## The Question

The roadmap recommends "Stripe Billing to start; Orb when you outgrow it." Given that the [`azure-proposal.md`](./azure-proposal.md) names Orb as a load-bearing platform decision, this document tests that assumption against the actual pricing model in `notes/product-specs/accordli_platform_overview.md`:

- Solo Pro: $200/mo, 10 ARCs included
- Solo Gold: $400/mo, 25 ARCs included
- Small Team: $600/mo, 3 seats, 40 ARCs shared
- Large Team: $2,000/mo, 10 seats, 130 ARCs shared
- Extra Contract Pack: $100, 10 ARCs, 12-month expiration
- Refund policy: first 7 days, ≤2 contracts analyzed

Plus from the roadmap: two-phase reserve/commit/rollback metering, monthly-in-advance billing, Stripe as the payment processor, SOC 2 audit-evidence requirements.

The honest answer is: **Orb removes ~30% of the billing system, not 80%. The 30% is real, but at this pricing complexity, it can wait.**

---

## What Orb Does That's Genuinely Hard to Build

These are the bits where home-rolling tends to leak billing-correctness bugs that customers find before you do.

### 1. Proration across plan changes mid-period

Pro → Gold on day 17 of a 30-day cycle, with 6 ARCs already consumed of the old plan's 10, and a credit pack purchase last week — what does the invoice look like? What ARCs are "used"? Stripe Billing handles dollar-proration; it does not understand your usage units.

Writing this yourself means a state machine with a test matrix you'll never finish covering. The bug pattern is: customer upgrades, edge case in proration logic produces a $3.42 discrepancy, customer notices on invoice, you owe a refund + an apology + an engineering investigation.

### 2. Credit pack ledger with expirations and consumption order

The spec says credit packs expire 12 months after purchase and are not refundable. The consumption order on every analysis becomes:

```
1. Included monthly quota
2. Oldest non-expired credit pack
3. Next-oldest credit pack
4. Overage (charged at end of period)
```

Each plan change can leave residual credits. Each refund mutates the ledger. Each expiration is a scheduled event that has accounting consequences (deferred revenue → recognized revenue or breakage).

This is a real distributed-systems-shaped problem: the ledger must be append-only, idempotent under retry, deterministic on replay, and produce the same answer whether you compute it forward from genesis or sum the latest snapshot. Auditors will ask to see it.

**This is the strongest single argument for Orb at any scale.** It's also the subsystem most worth paying for if Orb offers a startup-tier discount.

### 3. Plan versioning without grandfathering branches in code

You will change pricing within the first 12 months. When Gold goes from $400/25 ARCs to $450/30 ARCs, existing customers stay on the old plan until they choose to move. Orb stores plan versions; without it you end up with `if customer.plan_v == 1 { ... } else if v == 2 { ... }` branches everywhere, which never fully gets cleaned up and slowly poisons the codebase.

### 4. Custom contracted enterprise pricing

The spec explicitly carves out enterprise pricing as case-by-case. Each enterprise deal becomes a unique price book:

> "200 ARCs/mo, $1,500 base, 10% volume discount above 50 seats, NET-30, $5K signing credit, 24-month term, custom MSA terms on liability cap."

Orb has UI for this; in code it's a feature flag farm that grows linearly with deal count. Five custom enterprise contracts is the breaking point — pre-Orb you can survive on JSONB columns and code branches; post-Orb is a config change.

### 5. Invoice line-item rendering that explains itself

An invoice that reads:

```
Gold plan (Apr 2026)                      $400.00
  25 ARCs included, 28 used.
  3 ARCs overage @ $10                    $ 30.00
  Less 3 ARCs from Extra Contract Pack #2
    (purchased 2026-01-12)                -$30.00
  Sales tax (CA, 8.875%)                  $ 35.50
                                          --------
Total                                     $435.50
```

…is non-trivial to generate, must reconcile to the cent against the ledger, must survive plan changes mid-period, must be re-renderable for historical periods after pricing changes, and is the document a paying lawyer will scrutinize first.

### 6. Dunning state machine

Failed payment → in-app banner + email 1 day 0 → email 2 day 3 → retry day 5 → retry day 9 → grace period start day 10 → suspension day 14 → win-back email day 30. Stripe Smart Retries does the retry mechanics; the orchestration around it (in-app surfaces, suspension flow, automatic re-instatement on payment, audit trail of every transition) is yours either way, but Orb makes it a config rather than code.

### 7. Per-customer gross margin and revenue analytics

Solvable with SQL, but Orb gives it as a side-effect of being the system of record. Specifically valuable: cohort retention by plan, MRR/ARR/NRR with churn buckets, gross-margin-per-org views that include LLM cost from Helicone joined to revenue from Orb.

You will build a Metabase dashboard either way; Orb just shortens it.

---

## What Orb Does *Not* Do That You Still Build

Don't get lulled by the marketing. Orb is the *billing* engine, not the metering engine. You build all of this regardless:

- **Real-time entitlement enforcement.** Orb's view of usage lags by seconds-to-minutes. The request-path "can org X analyze right now?" check is local Postgres + your own logic. Orb is *eventual* truth on usage for billing, not *immediate* truth for enforcement.
- **Two-phase Reserve / Commit / Rollback** wrapping every billable operation. Orb takes commit-time events; the reservation pattern is yours.
- **`usage_events` append-only table.** Orb wants events pushed to it; you keep the source-of-truth copy locally for audit and reconciliation. (Orb's UI is convenient but Orb is not a substitute for your own immutable event log.)
- **Webhook handlers + monthly reconciliation jobs** to confirm Orb's ledger and yours agree. Disagreements are bugs you must catch.
- **Tenant-aware audit log of billing events** for SOC 2.
- **The actual payment processing.** Stripe sits behind Orb either way; the 2.9% + 30¢ doesn't go away.
- **The customer-facing usage dashboard.** Orb has one, but it's generic; lawyers want it inside Accordli's UI.
- **Soft and hard limit handlers, spend caps, in-app upgrade CTAs.** Product UX, not billing infrastructure.

So Orb removes ~30% of the billing system, not 80%.

---

## What's *Easy* in Your Model That Orb's Pricing Assumes Is Hard

Accordli's pricing is meaningfully simpler than most usage-metered SaaS, and Orb is priced for the harder cases. Specifically:

| Dimension | Accordli | Orb's typical customer |
|---|---|---|
| Usage units | **One** (ARC) | Multi-unit (API calls + tokens + storage + bandwidth + ...) |
| Overage pricing | **Flat** ($10/ARC) | Tiered, graduated, volume-discounted |
| Granularity | **Whole numbers** | Fractional (e.g., per-token) |
| Billing cadence | **Monthly-in-advance** | Mix of monthly, quarterly, annual prepay, true-up |
| Refund logic | **Mechanical** ("first 7 days, ≤2 analyses") | Workflow engine territory |
| Plan count | **4 + enterprise** | Often 10+ |

A solid Go engineer can build the metering core for this model in 2–3 weeks:

- `usage_events` table (append-only, idempotency-keyed, billing-period-stamped)
- `billing_periods` table (per-org, plan snapshot at period start)
- `credit_ledger` table (append-only, with expirations)
- Entitlement service (pure function over events + ledger + plan)
- Period rollover job (River cron)
- Stripe webhook handlers (subscription state mirror, idempotent on event ID)

**The hard 30% is what I listed in the previous section.** Most of it is the credit pack ledger.

---

## What Orb Costs

Public list pricing as of early 2026:

- **Entry production tier:** ~$720/mo flat
- **Revenue-share variant:** ~0.5–0.8% of billed revenue, often with a $500/mo floor
- **Startup program:** $0 for ~12 months for early-stage companies that apply and qualify

For Accordli at starter scale ($10K MRR target):

| Path | Effective monthly cost |
|---|---|
| Orb Startup Program (if accepted) | $0 |
| Orb revenue-share at 0.6% of $10K MRR | ~$60 (more likely $500 floor → $500) |
| Orb entry tier flat | $720 |
| Stripe Billing + in-house ledger | $0 incremental (Stripe processing fees exist on either path) |

The startup-tier number is the only one that makes Orb a no-brainer at this scale. **Apply first; decide based on the answer.**

---

## Alternatives to Orb

Reframe the question: I argued you shouldn't hand-roll **the credit pack ledger specifically** — Stripe Billing already gives you the rest of billing (subscriptions, dunning, invoicing, tax). So the real comparison is: **what gives us a credit pack ledger + invoice line items + plan versioning, cheaper or easier than Orb?**

Three real alternatives. Two are interesting; one is a trap.

### 1. Lago — the strongest alternative

Open-source Orb competitor. Same conceptual surface — metering, subscriptions, credit packs, plan versioning, invoice rendering, Stripe behind it.

- **Cloud free tier:** free up to ~$1M annual revenue. *Genuinely free*, not "free for 12 months." Above that, paid tiers start ~$400/mo and scale on revenue.
- **Self-hosted:** Apache 2.0, run it on your own Container Apps replica next to the worker. Free in dollars, costs ops time. Realistic if you want zero vendor lock and have the appetite to operate it.
- **Feature parity for our use case:** credit pack support with expirations ✅, plan versioning ✅, proration ✅, invoice rendering ✅, multi-tenant ✅. Less polished than Orb on enterprise quoting and analytics dashboards.
- **Trade-off:** smaller company than Orb, smaller customer base, less mature integrations directory. The risk is the standard infra-startup risk — they could fold or be acquired in a way that disrupts you. The self-host escape hatch meaningfully blunts that risk.

**Verdict:** if Orb's startup program rejects us, **Lago Cloud free tier is the strongest replacement.** ~80% of what Orb does, $0 in the relevant revenue band, with self-host as a hedge.

### 2. OpenMeter + Stripe Billing — the hybrid

Treat metering and subscriptions as separate problems and use the best tool for each.

- **OpenMeter:** OSS event ingestion, aggregation, idempotency, windowed sums. Cloud free tier exists. Solves the *eventing* hard parts.
- **Stripe Billing:** subscriptions, plans, dunning, invoicing, Stripe Tax, customer portal, dollar-proration. Already familiar.
- **You write:** the credit pack ledger and the entitlement service. OpenMeter doesn't have credit packs natively; Stripe's "promotion credits" don't really fit packs-with-expirations.

**Cost:** OpenMeter Cloud is roughly $0–$200/mo at our scale; Stripe is just payment processing fees. Total incremental: ~$0–$200/mo.

**Trade-off:** you keep ownership of the ledger. With OpenMeter handling event ingestion and idempotency, the ledger you write is much smaller (~500 lines of Go instead of ~1,500 if you also have to build the metering plumbing). The credit pack code becomes the only billing code you maintain — and it's the one piece you'd *want* to own anyway, since it's the part most coupled to your product semantics.

**Verdict:** good middle ground if maximum Stripe-familiarity matters and we don't mind owning a small, focused ledger.

### 3. Chargebee, Recurly, Zuora — the trap

Traditional subscription billing platforms. Cheaper than Orb (~$250–$600/mo), more mature, more enterprisey UI.

- **Why they look attractive:** Chargebee in particular has credit packs, plan versioning, dunning, decent invoice rendering, lots of payment-gateway integrations.
- **Why they're wrong for us:** built around "subscriptions with seats and a few add-ons." Usage metering is bolted on and clunky. Custom usage units (ARCs), real-time entitlement consumption from a unified balance (subscription quota + multiple credit packs), per-event metering — all are second-class citizens.
- **The bug pattern:** six months in we hit a usage edge case the system can't model cleanly, and end up doing exactly the home-rolled ledger work we were trying to avoid, *plus* paying Chargebee.

**Verdict:** skip. Wrong shape for Accordli's pricing model, even though the price tag looks friendly.

### Honorable mention: Paddle (merchant of record)

Different category — Paddle is the seller of record, takes ~5% of GMV, handles all tax compliance globally. Saves real headaches at small scale.

**Why not for Accordli:** B2B legal customers expect a clear vendor relationship. Their procurement asks for a DPA, an MSA, vendor-risk forms, a W-9 with *our* EIN. "Actually, Paddle is the merchant" is awkward, and lawyers will notice the credit-card-statement line item is "Paddle.com" not "Accordli." MoR fits prosumer SaaS; it doesn't fit selling to legal.

### Cost comparison at starter scale ($10K MRR)

| Path | Incremental monthly cost | Credit pack ledger | Plan versioning | Vendor lock-in |
|---|---|---|---|---|
| Orb Startup Program | $0 (12 months) | ✅ vendor | ✅ vendor | High |
| Orb entry tier | ~$720 | ✅ vendor | ✅ vendor | High |
| **Lago Cloud free tier** | **$0 (up to ~$1M ARR)** | ✅ vendor | ✅ vendor | Low (OSS, can self-host) |
| OpenMeter + Stripe Billing | ~$0–$200 | ✋ home-roll (small) | ✅ Stripe + ✋ home-roll | Low |
| Chargebee | ~$250–$600 | ⚠️ awkward fit | ✅ vendor | Medium |
| Stripe Billing only + full home-roll | $0 | ✋ home-roll (large) | ✋ home-roll | None |

---

## The Recommendation

Given Accordli's specific pricing model and starter scale, Orb's high-value features land in this priority order:

| Feature | Likelihood you actually need it in year 1 |
|---|---|
| Credit pack ledger with expirations | **High** — it's in the spec, day one |
| Invoice line-item rendering | **High** — lawyers scrutinize the first invoice |
| Plan versioning | **Medium** — you will change prices within 12 months |
| Proration on plan changes | **Medium** — depends on self-serve upgrade volume |
| Custom enterprise contracts | **Low → High** — flips the moment a $50K ACV deal closes |
| Dunning state machine | **Medium** — Stripe Smart Retries covers a lot |
| Per-customer analytics | **Low** — Postgres + Metabase suffices for a while |

### Decision (pending team discussion)

Leading candidates, in priority order. Final call to be made with the team — current lean is **Lago Cloud free tier** as the most pragmatic option that doesn't depend on getting accepted into someone else's startup program.

1. **Lago Cloud free tier** *(current lean)*. Free up to ~$1M ARR, covers the load-bearing credit pack ledger, plan versioning, invoice rendering. OSS heritage means we can self-host as a fallback if their cloud changes terms or the company changes hands. **No application required, no time-limited free window.**
2. **Orb Startup Program** *(only if accepted)*. $0/mo for 12 months removes the question entirely and pre-solves the credit pack ledger. Application-gated; if rejected, this option doesn't exist for us.
3. **OpenMeter + Stripe Billing + small in-house credit ledger.** ~$0–$200/mo. Maximum Stripe-familiarity, minimum vendor surface, but we own the credit pack code (which is the part you'd most want to own anyway).
4. **Stripe Billing alone + full home-rolled metering + ledger.** Only if 1–3 are unavailable. ~1,500 lines of Go and a high paranoia bar, particularly on the credit pack ledger.

**Migration triggers** apply on any of paths 1, 3, or 4 — i.e., the conditions under which we should reconsider Lago/OpenMeter/home-roll and move to Orb (or beyond):

1. First enterprise deal with custom pricing beyond "10% off list."
2. First pricing change we want to grandfather without code branches.
3. Credit pack purchases reach a volume where audit tooling matters (~$20K/mo of credit pack revenue).
4. Adding a second usage dimension (e.g., per-clause analysis, premium model tier).

Until any of those is true, the value of Orb at full price is mostly **insurance against billing bugs we'd otherwise discover by writing them.** That insurance is real but is largely also offered by Lago at $0 in our revenue band.

### The one thing not to home-roll, even at starter scale

The **credit pack ledger with expirations and FIFO-by-expiry consumption ordering** is the subsystem to be most paranoid about, because:

1. Bugs become customer-visible refund disputes.
2. The expiration logic touches deferred revenue accounting (auditors care).
3. The "which credit pack got drawn down" question is the kind of thing customers ask their accountant to verify.
4. It's the most complex piece of state in the whole billing system.

If you home-roll it, do these four things from day one and never compromise:

- **Append-only ledger.** No mutating updates. A consumption event subtracts a quantity by inserting a negative-delta row, never by editing the parent purchase.
- **Idempotency keys** on every write, scoped by `(org_id, source, source_id)`.
- **Deterministic replay.** A `compute_balance(org_id, as_of)` function is a pure fold over the ledger and produces the same answer at any time.
- **Reconciliation cron.** A daily job that recomputes balances from genesis and asserts equality with the latest cached snapshot. Page on disagreement.

Those four properties — append-only, idempotent, deterministic, reconciled — are the same properties Orb's ledger has. If you build them yourself, build them like Orb did.

---

## Closing Note

Orb is good software, sold at a price that assumes harder pricing models than Accordli currently has. The right answer is not "use Orb because the roadmap says so" — the roadmap was written before Lago's free tier was on the table at this scale. Today the decision tree is closer to:

- **Lago Cloud free tier** unless we have a strong reason to reach for Orb.
- **Orb** if their startup program accepts us (free) or if we hit one of the four migration triggers later.
- **OpenMeter + Stripe** if the team prefers minimum vendor surface area and we're comfortable owning the credit pack ledger.

This decision should be revisited at month 6 regardless. Pricing models calcify fast, and we want this question settled before the first enterprise customer asks for custom terms.

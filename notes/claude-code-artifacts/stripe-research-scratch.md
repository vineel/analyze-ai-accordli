# Stripe Research — what we need to know for the Accordli MVP

**Status:** working scratch doc. Captures (a) what Accordli will actually use Stripe for, (b) the Stripe primitives we need to understand to build that, (c) the open questions worth researching, and (d) findings as research lands. Iterative — keep appending.

---

## 1. Where Stripe fits in our architecture (synthesized from existing docs)

The roadmap and the Orb deep-dive together tell a consistent story:

- **Stripe is the payment processor and the subscription/invoicing engine in any path we pick.** Even if we run Lago, Orb, or our own ledger above it, Stripe is the thing that holds the card, charges it, and produces the legal invoice document. The 2.9% + 30¢ doesn't change.
- **The current leading billing-platform pick is Lago Cloud (free tier) sitting in front of Stripe.** Orb is a fallback if their startup program accepts us. Pure Stripe-Billing-only is the worst-case path.
- **What stays ours regardless of the billing platform:**
  - `usage_events` (append-only, idempotent, source of truth)
  - `credit_ledger` (append-only, FIFO-by-expiry consumption)
  - Two-phase Reserve / Commit / Rollback wrapping every ARC-consuming operation
  - Real-time entitlement enforcement (request-path, local Postgres)
  - In-app usage dashboard and limit handlers
- **What Stripe owns end-to-end:**
  - Card / payment-method storage (PCI scope reduction)
  - Subscription lifecycle and recurring charges
  - Invoice rendering (legal document)
  - Sales tax via Stripe Tax
  - Dunning retries (Smart Retries)
  - Customer Portal (payment method updates, invoice history)
- **What sits on the seam between us and Stripe (or Lago→Stripe):**
  - Plan / price catalog (mirrored both places)
  - Subscription state (Stripe is authoritative; we mirror locally)
  - Webhook ingestion and idempotent application
  - Refunds and credit notes

The Accordli pricing model that all of this has to support:

- 4 published plans + Enterprise. Solo Pro / Solo Gold / Small Team / Large Team.
- Each plan = monthly fee + included ARCs (monthly-in-advance billing).
- $100 ARC packs (10 ARCs, 12-month expiration, FIFO consumption after monthly quota).
- Refund window: first 7 days, ≤2 contracts analyzed (revision pending — see overview spec).
- Overage at $10/ARC (implied by the existing pack pricing; needs explicit confirmation).
- Team plans share ARCs across the Org.
- Mid-period plan changes need to do something sensible with already-consumed ARCs and remaining-quota ARCs.

---

## 2. The Stripe building blocks we need to understand

Grouped by what they enable. Everything below is a thing we will either use directly, configure, integrate with via webhook, or explicitly decide not to use.

### 2.1 Core objects (the API surface)
- **Customer** — one per Organization. Mirrors WorkOS Organization. Holds card, address, tax ID.
- **Product / Price** — Stripe's plan catalog. We will have one Product per plan, with one or more Prices (monthly USD).
- **Subscription / SubscriptionItem** — links Customer to Price(s). Drives recurring billing. Supports licensed (flat) and metered items as separate items on the same subscription.
- **Invoice / InvoiceItem** — generated at period end (or proration events). The actual document the customer gets.
- **PaymentIntent / SetupIntent** — represent attempts to charge / save a card. SetupIntent is what you use during sign-up to attach a card without charging immediately.
- **PaymentMethod** — a stored card (or other instrument) attached to a Customer.
- **Charge** — legacy-ish, still there. Don't reach for it directly; use PaymentIntent.
- **Refund / CreditNote** — two different things; we likely need both.
- **TaxRate / Stripe Tax** — Stripe Tax is the modern automatic option; manual TaxRate is the older API.
- **Coupon / PromotionCode / Discount** — for offering discounts and promo codes.
- **Event / Webhook** — every state change emits an event; we ingest via webhook.

### 2.2 Hosted UX surfaces (the "let Stripe build it" options)
- **Stripe Checkout** — hosted, redirect-style, complete-flow page. Pros: easy. Cons: less branded, slightly disconnected feel.
- **Embedded Components / Elements** — JS components you embed in your own React UI. More work, more control, more like "your app."
- **Customer Portal** — hosted page for customers to update card, see invoices, cancel subscription, etc. **Free** with Stripe Billing. Configurable feature set.
- **Pricing Tables** — hosted plan picker; probably not the right fit for a B2B-with-account-creation flow but worth considering for a marketing site.

### 2.3 Billing model primitives
- **Licensed pricing** — flat monthly fee. Maps to subscription base.
- **Metered pricing** — Stripe calls it "usage-based" — you push usage events to a SubscriptionItem and Stripe rolls them into the next invoice. Comes in flat / tiered / volume / graduated flavors.
- **Tiered pricing** — graduated or volume tiers within a Price. Maps to N-included-then-overage style if we go pure Stripe.
- **Billing thresholds** — Stripe can auto-trigger an interim invoice when usage crosses a $ threshold. Useful for runaway-usage protection.
- **Proration behavior** — `create_prorations`, `none`, `always_invoice`. Affects what happens on plan changes mid-period.
- **Backdating / billing cycle anchor** — controls when the period starts.
- **Trials / trial_end** — usable if we want a trial; not in the current spec.

### 2.4 Tax
- **Stripe Tax** — automatic calculation across US states (and intl) based on customer address. Charges a per-transaction fee. Handles registration tracking and threshold monitoring.
- **Sales tax on legal SaaS in the US is non-uniform.** Some states tax SaaS, some don't, some only if "delivered electronically," etc. Stripe Tax abstracts this but registration is still on us per state we cross threshold in.

### 2.5 Identity / fraud / compliance
- **Stripe Radar** — fraud detection, on by default.
- **3D Secure / SCA** — required for EEA and increasingly common for US issuers. Stripe handles the flow but the integration has to surface it correctly.
- **PCI scope** — using Stripe Elements / Checkout keeps us out of scope for SAQ-D. Important for SOC 2 narrative.
- **Stripe data residency** — Stripe stores data in the US by default; EU residency exists for some products but is opt-in/contracted.

### 2.6 Engineering surfaces
- **Stripe Go SDK** (`stripe-go/v82` is current). Idiomatic enough; auto-paginated iterators; idempotency keys are a per-request option.
- **Stripe CLI** — local webhook forwarding (`stripe listen --forward-to localhost:...`), event replay, fixture creation.
- **Idempotency keys** — every state-changing API call should pass one; we generate, Stripe deduplicates for 24h.
- **Webhook signing** — `Stripe-Signature` header, HMAC verification, replay protection via timestamp tolerance.
- **Test mode vs live mode** — fully separate; objects don't migrate. Important: separate webhook endpoints, separate API keys, separate event streams.
- **Restricted API keys** — scoped keys; relevant for SOC 2 (least privilege from app code).

### 2.7 What Stripe does NOT do well for us (so we know not to lean on it)
- ~~Credit packs with expirations and FIFO consumption.~~ **Updated:** Stripe now ships **Billing Credits / Credit Grants** with `expires_at` and priority-ordered consumption. Models our pack-with-expiry shape natively. See §4.2 / §9.1. (The original line — "Stripe has customer balance and promotion credits but neither models packs-with-expiry cleanly" — was true before this primitive shipped.)
- **Plan versioning with grandfathered customers.** Stripe lets you keep old Prices around, but the bookkeeping ("v1 subscribers stay on v1 forever; v2 is the new default") is yours to enforce.
- **Real-time entitlement.** Stripe's view of usage lags by minutes-to-hours; the request-path "can this org analyze right now?" check must be local.
- **Custom enterprise pricing at scale.** Per-customer Prices work for a handful. Stripe Quotes (§9.7) helps for the formal-quote step but the per-customer Price proliferation is still ours to manage.
- **Subscription updates from the Customer Portal when the subscription has metered items.** Cancel works; price-change does not. We own the upgrade/downgrade UX. See §9.4.
- **Auto-granting included monthly quota at renewal.** If we model the included ARCs as Credit Grants, Stripe does *not* auto-grant on each renewal — we have to call the Credit Grants API ourselves on each invoice cycle. See §9.1.

---

## 3. Open questions to research

Numbered for later cross-referencing. Roughly priority-ordered.

### 3.1 Core product/pricing modeling
1. **What's the current canonical Stripe pattern for "monthly fee + N included usage units + overage"?** Is it a single Price with graduated tiers, or two SubscriptionItems (one licensed, one metered)? What does the invoice look like in each shape?
2. **How does the new "Stripe Billing usage-based / Meters" API (introduced 2024) compare to the legacy `usage_records` API?** Which one should we build on in 2026?
3. **Mid-period plan changes** — does Stripe's proration handle "X ARCs already used this period" sensibly, or is that always our job?
4. **Refund vs. credit note vs. customer balance** — which do we use for the 7-day refund? Which for "we ate this ARC because the lens failed"? Implications for revenue recognition?
5. **Sales tax on contract-analysis SaaS in the US** — is Stripe Tax sufficient, or do we need a tax advisor to map our service to state tax codes? What's the registration threshold trigger?

### 3.2 Sign-up and purchase flow
6. **Stripe Checkout vs. Embedded Components** for a B2B sign-up flow that needs to also collect Org info and pick a plan? Industry default in 2026?
7. **How do we handle the "credit card collected at sign-up but first month not yet charged" state?** SetupIntent then attach to Subscription? Or charge immediately?
8. **Self-serve credit-pack purchase** — one-shot PaymentIntent? Stripe Checkout in subscription mode + one-time line item? How does this interact with our `credit_ledger`?
9. **Failed-payment grace period UX** — does Stripe have a built-in "subscription is past_due, now do these things" workflow we can lean on, or do we orchestrate it ourselves on top of webhooks?

### 3.3 Integration architecture
10. **If we go Lago Cloud, how does Lago talk to Stripe?** Does Lago create the Stripe Customer or do we? Where does the subscription actually live? Who is authoritative for what?
11. **If we go pure Stripe Billing first and migrate to Lago later, what's the migration path?** What gets duplicated, what has to be reconciled, what breaks?
12. **Webhook reliability** — Stripe's at-least-once delivery + signing + ordering guarantees. What's the standard idempotent-handler shape in Go?
13. **Reconciliation cron** — sum of our `usage_events` for period P should match the metered-usage line on Stripe invoice for period P. How do we run this and what do we alert on?

### 3.4 Account / customer model
14. **One Stripe Customer per Org, or per User in solo cases?** Is there any reason to ever have more than one Customer per Org?
15. **Tax ID collection** — for B2B customers (firms with EINs), what's the right collection point? Customer Portal supports it; is sign-up the right time?
16. **Multi-currency / international cards** — out of scope for MVP per spec, but does choosing the wrong objects now lock us in?

### 3.5 Operational / SOC 2
17. **Stripe sub-processor relationship** — what's on their DPA? Is it auto-applied or do we sign? What's their SOC 2 / PCI attestation availability for our trust page?
18. **Data residency** — can a customer demand "all Stripe data about us stays in the US"? Default behavior?
19. **Audit-evidence shape** — what does Stripe export look like for our SOC 2 audit? Is there a built-in export of all subscription state changes?
20. **Test/live mode hygiene** — separate accounts? Single account with mode flag? Implications for branch deploys / staging.

### 3.6 Specific to our pricing model
21. **Refund-window enforcement** — does Stripe support "auto-refund if conditions met" or is that always our app logic calling the Refund API?
22. **Spend caps** — can org admins set "don't charge me more than $X this month"? Stripe billing thresholds are the closest primitive; do they fit?
23. **Team-plan seat enforcement** — if Small Team includes 3 seats, do we model seats as a Stripe quantity on a SubscriptionItem (and pay overage automatically) or enforce in-app and treat seats as a fixed plan attribute? Implications for SCIM auto-add.
24. **Quote / order-form workflow for Enterprise** — Stripe has a Quotes API; is it any good or do we use Docusign + manual subscription creation?

### 3.7 Roadmap / horizon questions
25. **What pricing changes are realistic within 12 months that would make us want to switch off Stripe-only and onto Lago/Orb?** (Helps decide migration triggers.)
26. **Is there anything Stripe announced in 2025–early 2026 that materially changes this calculus?** (E.g., native credit-pack support, native plan versioning, deeper metering.)

---

## 4. Research findings (filled in as we go)

### 4.1 Metering: legacy `usage_records` is gone — use Meters

The old `Subscription Item Usage Records` API was **removed** in Stripe API version `2025-03-31.basil`. Every metered Price now requires a backing **Meter**. Anything we build in 2026 must be on Meters.

Practical differences vs. the legacy API:

- Meter events are reported per **Customer ID** (not per SubscriptionItem ID). A Meter can track usage across multiple customers. Looser coupling between metering and subscription state — closer to the shape we want anyway.
- Meters provide a **1-hour grace period** for events recorded after invoice creation, and a **24-hour cancellation window** for events already sent. Mistakes are recoverable; the legacy API was strict-current-period only.
- Designed for high-throughput ingestion and aggregation (mean / sum / count / last / max windowed).

**Implication for Accordli:** the natural shape is to push one meter event per ARC commit (in our `Commit` step of Reserve/Commit/Rollback), keyed on the Stripe Customer ID we mirror onto each Org. We never need to know the SubscriptionItem ID. ([Stripe — Migrate to billing meters](https://docs.stripe.com/billing/subscriptions/usage-based-legacy/migration-guide), [Stripe — Removes legacy usage-based billing changelog](https://docs.stripe.com/changelog/basil/2025-03-31/deprecate-legacy-usage-based-billing))

### 4.2 Stripe Billing Credits — could plausibly replace Lago for credit packs

This is the biggest finding. Stripe shipped **Billing credits** in 2024–2025 and it materially changes the Orb deep-dive's reasoning. The TL;DR from the Orb doc was: "the credit pack ledger is the load-bearing reason to consider Lago/Orb." Stripe now has a native primitive for it.

What it does:

- **Credit Grants** are records of prepaid (or promotional) credits attached to a Customer.
- They support **`expires_at`** (12-month-from-purchase is a one-line config).
- **Consumption order** when multiple grants exist is determined by:
  1. priority number (you set it)
  2. expiration date
  3. category (promotional vs. paid)
  4. effective date
  5. creation date

  That's **functionally FIFO-by-expiration** with override knobs — exactly the consumption order our spec calls for.

- They **only apply to metered prices on subscriptions**. Cannot pay a licensed flat fee. Fits our model (ARC overage is metered; the monthly base fee is licensed and is paid normally).
- Credits apply **after discounts but before taxes**.
- Hard limit: **max 100 unused credit grants per customer.** For a solo lawyer buying a $100 pack a month that's 8+ years of headroom; for a firm buying multiple packs per week it's a constraint to watch. Worth flagging.
- Cannot be applied to one-time invoice items added to a previewed invoice (e.g., setup fees) — irrelevant to our model.

Sales flow per the Stripe implementation guide:

1. Create a one-off invoice for the credit pack purchase (or use Checkout in payment mode).
2. Listen for `invoice.paid`.
3. Call **Credit Grants API** to grant N credits with `expires_at = now + 365d`, priority set so older packs drain first.
4. Customer's metered ARC overage on the next invoice automatically draws down the grant before charging the card.

**What this changes:** the credit-pack-ledger argument for Lago/Orb gets weaker. **It does not eliminate the case** — we still want our own internal ledger for SOC 2 audit trail, real-time entitlement, and as a reconciliation source of truth — but the question shifts from "how do we build a credit-pack ledger" to "how do we mirror Stripe's credit grant state into our local ledger and reconcile."

**Pressure-test before relying on this:** the 100-grant ceiling, exact priority-ordering semantics under partial consumption mid-period, and behavior under refunds (does refunding a credit-pack purchase auto-expire the grant?) — all need to be verified before we commit. ([Stripe — Set up billing credits](https://docs.stripe.com/billing/subscriptions/usage-based/billing-credits/implementation-guide), [Stripe — Billing credits](https://docs.stripe.com/billing/subscriptions/usage-based/billing-credits))

### 4.3 The "monthly fee + N included + overage" pricing pattern

Two viable shapes inside Stripe:

**Shape A — two SubscriptionItems on one Subscription** (the documented standard):
- Item 1: licensed Price for the flat monthly fee ($200 / $400 / $600 / $2000).
- Item 2: metered Price tied to a Meter (`arc_usage`), with a **graduated tier** — first N units at $0, every unit beyond at $10.
- Single invoice at period end shows both lines.

**Shape B — Billing Credits as the "included quota"**:
- Item 1: licensed Price for the flat monthly fee.
- Item 2: metered Price at $10 per ARC, no free tier.
- Each renewal automatically grants a fresh "monthly quota" Credit Grant with `expires_at = period_end` (uses the Stripe credit system to model the included ARCs).
- Customer's credit balance covers the first N ARCs of the month before the card is charged for overage.
- Add-on packs are *additional* Credit Grants with longer expiry.

Shape B is conceptually cleaner — it makes "monthly included ARCs" and "purchased pack ARCs" the same primitive, just different expiry. But it ties harder to the Credit Grants API and we'd need to confirm Stripe automates "grant N credits at every renewal." If they don't, we'd be writing that orchestration ourselves on the `invoice.created` webhook.

Shape A is the safer default; Shape B is what we'd test if we wanted to go all-in on the Stripe-native path. ([Stripe — Flat fee and overages use case](https://docs.stripe.com/billing/subscriptions/usage-based-v1/use-cases/flat-fee-and-overages), [Stripe — Recurring pricing models](https://docs.stripe.com/billing/subscriptions/usage-based/pricing-models))

### 4.4 Sign-up flow: SetupIntent vs PaymentIntent

For Accordli's "monthly in advance" billing, the first month is charged immediately on sign-up. So:

- **PaymentIntent with `setup_future_usage = off_session`** is the right shape — charges the first month and saves the card for renewals.
- **SetupIntent** is for trial-style flows where no money moves up front. Not our case (yet).

Stripe Checkout (subscription mode) handles this glue automatically — it picks the right intent based on the subscription's first-invoice amount and trial config. If we use Embedded Components, we're choosing the intent type ourselves. ([Stripe — Setup Intents API](https://docs.stripe.com/payments/setup-intents), [Stripe — Subscriptions overview](https://docs.stripe.com/billing/subscriptions/overview))

### 4.5 Customer Portal — what works, what's blocked

Stripe-hosted, free with Billing, configurable via Portal Configurations API. Works fine for:

- Update payment method (with new 2025-era `allow_redisplay` parameter for stored methods)
- Download past invoices and receipts
- Cancel subscription (with feedback collection if configured)
- Update billing address / tax ID
- View upcoming invoice estimate

**Important constraint: customers can cancel from the portal but not *update* a subscription that has metered usage or multiple products.** That hits us — every plan will have at least the licensed-fee item plus a metered-ARC item, and possibly a credit-pack mechanism layered on top. So plan changes (Pro → Gold) probably need a custom in-app flow, not the portal. Cancel can still be portal-driven.

This is a moderate UX hit, not a blocker. We were going to want a custom upgrade UX anyway (it's the highest-conversion moment in the funnel).

([Stripe — Customer management](https://docs.stripe.com/customer-management), [Stripe — Configure the portal](https://docs.stripe.com/customer-management/configure-portal))

### 4.6 Sign-up UX: Stripe Checkout vs Elements/Embedded Components

**Stripe Checkout** (hosted page or embedded form):
- Both variants are now first-class. Same backend, different rendering.
- Pre-built, handles SCA / 3DS / wallet methods / regional payment methods automatically.
- "Embedded Checkout" lives inside our domain; full Stripe-hosted is a redirect.
- Lowest engineering effort; least visual control.

**Stripe Elements** (Payment Element specifically):
- React components we drop into our own form.
- Maximum control over UX; we own the surrounding chrome.
- Still PCI-scope-friendly (card data goes direct to Stripe, never touches our server).
- More wiring (we orchestrate the Intent ourselves).

**Recommendation for the MVP**: start with **Embedded Checkout** for sign-up + plan upgrade + credit-pack purchase. Migrate the upgrade flow to Elements later if the conversion data justifies investing in a more bespoke UX. The marginal-design argument for Elements is real but small in the first three months when we're still figuring out funnel shape. ([Stripe — Build a payments page](https://docs.stripe.com/payments/checkout))

### 4.7 Lago ↔ Stripe seam (if we go that path)

Confirms what the Orb deep-dive assumed:

- **Lago owns**: subscription model, plans, invoices (Lago renders the actual invoice document), credit packs, plan versioning, usage metering.
- **Stripe owns**: card storage, payment processing, dunning retries.
- The seam: Lago generates an invoice → fires a `payment_intent` to Stripe → Stripe attempts the charge → fires webhooks back to Lago → Lago updates invoice state.

So with Lago, the invoice the customer sees is *Lago's* invoice, not Stripe's. The Stripe Customer Portal would only give them card management — they'd see invoices in our app (rendered by us from Lago data) or in a Lago-hosted portal.

That's a meaningful product-experience difference. The default Stripe Customer Portal "show me my last 12 invoices, click to download PDF" is one-line config; replicating that with Lago is build-it-yourself or use Lago's portal which is likely less polished.

**If we go Lago, we own the invoice-history UX too.** ([Lago — Stripe integration docs](https://getlago.com/docs/integrations/payments/stripe-integration))

### 4.8 Sales tax — Stripe Tax verdict for Accordli

The taxability picture for SaaS in the US in 2026:

- **~24–25 states tax SaaS in some form.** The split is roughly: Northeast + South lean toward taxing; West Coast + Midwest tend not to (rough heuristic, with notable exceptions — TX taxes as data processing service at 80% of price; NY, PA, CT, OH tax outright; CA, FL, MO, IL don't).
- **5 states have no statewide sales tax**: AK (some localities do), DE, MT, NH, OR.
- Some states view SaaS as **"intangible service"** akin to legal advice and exempt it. Worth checking whether Accordli's positioning ("legal AI agent producing legal-style work product") could get classified differently in some states than generic SaaS — could be in our favor or against.

Practical answer:
- **Stripe Tax handles the calculation and collection mechanics.** It also tracks our state-by-state economic nexus and warns us when we approach a registration threshold.
- **It does not register us in states.** Crossing a nexus threshold (typically $100K in sales or 200 transactions/year per state, varies) requires us to register with each state's revenue department.
- **It does not tell us *whether* our service is taxable in state X for our specific business model.** That's a tax-advisor question — and important enough for legal-vertical positioning to want a written opinion early. The classification ("are we SaaS, are we a digital service, are we a non-taxable professional service?") materially affects which states we owe.

**Action items**: turn on Stripe Tax from day one, flag for the next finance/tax-advisor conversation: "get a written taxability memo from a SaaS-experienced sales-tax advisor before the first $100K of revenue." ([Stripe — SaaS taxability in the US](https://stripe.com/guides/introduction-to-saas-taxability-in-the-us), [Numeral — Sales tax on SaaS state-by-state](https://www.numeral.com/blog/sales-tax-on-saas))

### 4.9 Spend caps — billing thresholds

- Stripe **deprecated** billing thresholds in `2025-03-31.basil`, then **un-deprecated** them in `2025-05-28.basil`. Real flip-flop in API stability. Currently usable.
- A subscription supports **one monetary threshold**. When the running total of metered usage on the current period reaches it, Stripe issues an interim invoice and resets the running total.
- Doesn't support a hard "stop at $X" — it issues an extra invoice at the threshold; usage above it still bills next period.

So: useful for **early warning / breaking up huge invoices**, not as a true spend cap. Spend caps as customers might think of them ("don't let me spend more than $X/mo on overage") still need to be enforced in our app at the entitlement layer. The Stripe primitive is supplementary, not sufficient. ([Stripe — Set up thresholds](https://docs.stripe.com/billing/subscriptions/usage-based/thresholds), [Stripe — Reintroduces billing thresholds](https://docs.stripe.com/changelog/basil/2025-05-28/reintroduce-billing-thresholds))

### 4.10 Compliance posture (for our trust page)

Stripe holds:
- **SOC 2 Type II**, refreshed annually
- **PCI DSS Level 1** (the highest tier; required for any processor doing >6M transactions/yr)
- **ISO 27001**

Documents available at **trust.stripe.com** (gated by NDA in some cases; the AoC is needed for our own PCI 12.8 third-party-service-provider file when we eventually do PCI scope work).

**Stripe DPA is auto-applied** under their Services Agreement — no separate signing. Customers ask for DPAs *from us*; we point downstream at our subprocessors, of which Stripe is one. Stripe maintains a public subprocessor list with **30-day advance notice** of additions if we subscribe to email updates.

What this gives our trust page on day one: the standard "We use Stripe to process payments. Card data never touches Accordli infrastructure. Stripe is SOC 2 Type II, PCI DSS Level 1, ISO 27001 certified." paragraph. ([Stripe — Security at Stripe](https://docs.stripe.com/security), [Stripe — DPA](https://stripe.com/legal/dpa))

---

## 5. Updated open questions (post-research)

Some of section 3 is resolved; new ones surfaced. This is the live list to take into a working session.

### Resolved (or clear enough)
- ~~Q1 (included + overage pattern)~~ — two SubscriptionItems (licensed + metered-with-tier), or use Billing Credits to model included quota. §4.3.
- ~~Q2 (Meters vs usage_records)~~ — Meters; legacy is gone. §4.1.
- ~~Q5 (sales tax sufficiency)~~ — Stripe Tax for mechanics; written opinion from a tax advisor for classification. §4.8.
- ~~Q6 (Checkout vs Embedded)~~ — Embedded Checkout for MVP, revisit. §4.6.
- ~~Q7 (card-at-signup state)~~ — PaymentIntent with `setup_future_usage = off_session` (or let Checkout handle it). §4.4.
- ~~Q22 (spend caps)~~ — billing thresholds are partial; real cap enforcement is ours. §4.9.

### Sharpened, still open
- **Q-new-A: Does Stripe Billing Credits make Lago unnecessary for us?** This is now the central question. The Orb deep-dive's "credit pack ledger is the load-bearing reason for Lago/Orb" argument may be partly dissolved. Worth a focused doc: *Stripe-only with Billing Credits* vs *Lago Cloud free* vs *Orb startup program*, with the credit-pack subsystem specifically re-evaluated. Suggested next research artifact.
- **Q-new-B: 100-credit-grant ceiling on a Customer.** Verify exact semantics — is it 100 *active* grants (drained ones don't count), or 100 *ever*? At firm scale (Large Team buying packs aggressively), how fast does this fill up?
- **Q-new-C: Refund of a credit-pack purchase.** Does refunding the original Stripe payment auto-expire/zero the corresponding Credit Grant? Or do we have to call Expire on the grant ourselves? Reconciliation correctness depends on this.
- **Q-new-D: Plan change UX.** Customer Portal can't change subscriptions with metered items. So our app owns the upgrade/downgrade UX. What does the proration look like — Stripe's `proration_behavior` flags, or do we cancel-and-recreate?
- **Q-new-E: Renewal-time monthly quota grant.** If we model included ARCs as Credit Grants, does Stripe re-grant every period automatically, or do we hook `customer.subscription.updated` / `invoice.created` and call Credit Grants ourselves on rollover?

### Still open from Section 3
- Q3 (proration on plan changes) — partly answered by Q-new-D; more spec work needed
- Q4 (refund vs credit note vs balance) — need a written rule for each case
- Q8 (self-serve credit pack flow end-to-end) — outline the API sequence
- Q9 (failed payment / dunning) — how much of grace period UX do we lean on Stripe Smart Retries vs build ourselves
- Q10/11 (Lago architecture, migration) — partly resolved by §4.7; full detail if we pick Lago
- Q12 (webhook idempotency in Go) — pattern doc to write
- Q13 (reconciliation cron specifics)
- Q14 (one Customer per Org confirmation)
- Q15 (tax ID collection point)
- Q16 (multi-currency)
- Q17/18 (data residency, EU customers)
- Q19 (audit-evidence shape from Stripe)
- Q20 (test/live mode hygiene)
- Q21 (refund window enforcement)
- Q23 (team-plan seat enforcement model)
- Q24 (Stripe Quotes for Enterprise — worth a brief look)
- Q25/26 (12-month roadmap pressure on platform choice)

---

## 6. Provisional architecture sketch (subject to revision)

Putting the findings together, the simplest defensible MVP architecture is:

```
┌──────────────────────────────────────────────────────────┐
│ Accordli app (Go API + worker)                           │
│  - usage_events       (append-only, our source of truth) │
│  - credit_ledger      (append-only; ours + mirror of     │
│                        Stripe Credit Grants)             │
│  - reservations       (mutable, TTL-bounded; in-flight   │
│                        holds during a ReviewRun)         │
│  - billing_periods                                       │
│  - entitlement service (Reserve check =                  │
│      ledger SUM(non-expired) - SUM(active reservations)) │
│  - Reserve / Commit / Rollback wrapper                   │
│  - webhook handler   (idempotent on Stripe event ID)     │
│  - reconciliation cron (ledger ↔ Stripe Credit Grants)   │
└────┬──────────────────────────────────────┬──────────────┘
     │ on Commit: meter event                │ webhook ingress
     │ on pack purchase: Credit Grant        │
     │ on renewal: monthly quota Credit Grant│
     ▼                                       │
┌──────────────────────────────────────────────────────────┐
│ Stripe                                                   │
│  - Customer per Org                                      │
│  - Subscription per Org                                  │
│     • Item 1: licensed Price (monthly fee)               │
│     • Item 2: metered Price → Meter "arc_usage"          │
│  - Credit Grants (monthly quota + purchased packs)       │
│  - Stripe Tax (auto)                                     │
│  - Customer Portal (cancel + payment method only)        │
│  - Checkout (sign-up + plan change + pack purchase)      │
└──────────────────────────────────────────────────────────┘
```

Note: `reservations` is intentionally separate from `credit_ledger` — see §8.2.
The ledger is the immutable accounting record; reservations are mutable lifecycle
state for in-flight ReviewRuns. The entitlement service reads both inside one
transaction at Reserve time so two parallel runs can't double-spend the last ARC.

This is a Stripe-only MVP. **It does not pre-decide against Lago/Orb** — if we hit the migration triggers from the Orb deep-dive (custom enterprise, a second usage dimension, plan grandfathering pain, ledger volume), we add a billing engine in front of Stripe and demote Stripe to "payment rails only."

The honest read of where the research has landed: **the Stripe-only MVP is more credible now than the Orb deep-dive assumed**, because Billing Credits exists. The hard parts that remain are reconciliation discipline and our own ledger. The hardest single risk is the 100-grant ceiling and the refund-of-pack-purchase semantics — confirm both before committing.

---

## 7. Suggested next steps

In rough priority order:

1. **Verify Billing Credits edge cases** that the Stripe-only path depends on: 100-grant ceiling, refund→grant expiration semantics, monthly-quota auto-grant at renewal. ~1 hour of testing in a sandbox + targeted doc reading.
2. **Write a short companion doc** comparing Stripe-only-with-Billing-Credits vs Lago-Cloud-free, since the orb-deepdive predates Billing Credits and its conclusion may shift.
3. **Get a written sales-tax classification opinion** for "AI-driven contract analysis service marketed to lawyers" before $100K MRR. Tax advisor work, not engineering.
4. **Sketch the webhook handler** for Stripe events as a small Go skeleton — proves the idempotency/ordering pattern early.
5. **Decide on sign-up UX**: Embedded Checkout for plan + payment method capture, vs full custom Elements form. Default Embedded Checkout unless someone has a reason.
6. **Decide on plan-change UX**: build in-app (because portal can't), confirm `proration_behavior` choice.

---

## 8. Credit ledger and reservations — design notes

Worked through in conversation; recording here so it doesn't get lost. This expands on the `credit_ledger` line item from the roadmap and the orb-deepdive's "the one thing not to home-roll" note.

### 8.1 What `credit_ledger` is

An **append-only event log** of every change to a customer's prepaid ARC balance. The balance itself is not a stored counter — it is always *derived* by folding the rows. Append-only + idempotent + deterministic-on-replay + reconciled-daily are the four properties to never compromise.

```
credit_ledger
  id               uuid    PK
  org_id           uuid
  delta            int                  -- +N for a pack purchase, -1 per ARC consumed
  reason           enum                 -- pack_purchase | consumption | expiration | refund |
                                        --   adjustment | monthly_quota_grant
  source           text                 -- 'stripe' | 'review_run' | 'manual_adjustment'
  source_id        text                 -- payment_intent_id, review_run_id, etc.
  source_grant_id  text                 -- on consumption rows: which Stripe Credit Grant
                                        --   the ARC was drawn from (mirrors Stripe's
                                        --   priority-ordered consumption); null on others.
                                        --   Required for refund-of-pack attribution (§14.4).
  expires_at       timestamp            -- only meaningful on positive (grant) rows
  idempotency_key  text    UNIQUE
  created_at       timestamp
```

**Balance read:**

```sql
SELECT COALESCE(SUM(delta), 0)
FROM credit_ledger
WHERE org_id = $1
  AND (expires_at IS NULL OR expires_at > now());
```

This sits behind an index on `(org_id, expires_at)` and is a sub-millisecond query at any realistic scale. Even a power-using firm generates only a few hundred rows per year.

The same shape supports the monthly plan quota (treated as a credit grant that expires at period end) and purchased packs (12-month expiry), unifying both into one ledger. Stripe Billing Credits models the same thing; if we use it, our ledger is a mirror + audit copy of Stripe's grant state, not the only ledger.

### 8.2 Reservations live in a separate table — the ledger records only actual consumption

**Two reasonable shapes; we pick the cleaner one.**

**Shape 1 (chosen): split the lifecycle from the accounting log.**

```
reservations (mutable, TTL-bounded)
  id              uuid   PK
  org_id          uuid
  review_run_id   uuid
  qty             int                       -- always 1 in the ARC model
  status          enum   (reserved | committed | cancelled | expired)
  expires_at      timestamp                 -- TTL for crashed-worker cleanup
  created_at      timestamp

credit_ledger
  (as above — append-only, never updated)
```

Why this split:

- The ledger is what auditors and reconciliation read. Reservations that get rolled back never spent anything; they shouldn't pollute the immutable accounting record.
- Reservations have maintenance churn (TTL sweeps, orphan cleanup). You don't want that in the accounting log.
- Reconciliation against Stripe Credit Grants is apples-to-apples — both record balance changes, not lifecycle events.

**Shape 2 (rejected): everything in one table as status events.** Reserve writes a tentative -1 row; Commit writes a "confirmed" row; Rollback writes a "voided" row. Collapses two tables into one but makes balance queries depend on knowing which rows count, and pollutes the audit record with non-events.

### 8.3 The lifecycle of one ReviewRun against the ledger

```
Reserve(org=O, run=R)
  Begin transaction (REPEATABLE READ minimum)
    available =
        SUM(credit_ledger.delta WHERE org=O AND non-expired)
      -
        SUM(reservations.qty   WHERE org=O AND status='reserved')
    if available < 1:  return 402 with upgrade CTA
    INSERT reservations (org, run, qty=1, status='reserved',
                         expires_at = now()+30m)
  Commit transaction
  → returns reservation_id

Run the ReviewRun (prefix step + parallel lenses, vendor failover, etc.)

Commit(reservation_id)                  // ≥90% of lenses completed
  Begin transaction
    UPDATE reservations SET status='committed' WHERE id=...
    INSERT credit_ledger (
        org, delta=-1, reason='consumption',
        source='review_run', source_id=run_id,
        idempotency_key=run_id)         -- dedupes a double-commit
  Commit transaction

Rollback(reservation_id)                // failed prefix, <90% partial, etc.
  UPDATE reservations SET status='cancelled' WHERE id=...
  // no ledger row — no consumption happened, so nothing to record
```

**Ledger writes per ReviewRun outcome:**

| Outcome                          | reservations row | credit_ledger row  |
|----------------------------------|------------------|--------------------|
| Completed (≥90% lenses)           | committed        | one -1 row         |
| Partial (<90% lenses)             | cancelled        | none               |
| Failed (prefix step never worked) | cancelled        | none               |
| Crashed worker (TTL expiration)   | expired (sweep)  | none               |

Lines up exactly with the ARC Consumption rules in `Reviewer-v2.md` — only fully-completed runs consume an ARC, and they consume exactly one regardless of how many user-initiated retries the Review later sees.

### 8.4 Pack-purchase write path (recap)

For completeness, the inverse direction — money in, balance up:

```
1. User clicks "Buy 10 ARCs"
2. Backend opens Stripe Checkout (mode=payment, $100 line item)
3. User pays
4. Stripe → POST /webhooks/stripe → handler verifies signature
5. Dedupe on event.id via processed_webhooks table
6. In one transaction:
     INSERT credit_ledger (delta=+10, reason='pack_purchase',
                           source='stripe',
                           source_id=payment_intent_id,
                           expires_at=now()+365d,
                           idempotency_key=event.id)
7. Call Stripe Credit Grants API to mirror the grant onto the
   Stripe Customer (Idempotency-Key header = event.id)
8. Return 200
```

Two writes (our ledger + Stripe's grant) are not atomic. The webhook handler returns 5xx if either fails so Stripe retries; the daily reconciliation cron is the safety net for any drift.

Refunds are the inverse and are the trickiest case: half the pack may already be consumed. Specced separately when we get there.

### 8.5 Reconciliation cron

Daily job, two checks:

1. **Internal consistency**: re-fold the ledger from genesis for each org and assert equality with the cached `current_balance` view. Page on disagreement.
2. **External consistency vs Stripe**: for each org, sum non-expired Credit Grants on the Stripe Customer and compare to our ledger balance. Tolerate small lag (Stripe is eventual); page on persistent drift > N minutes.

Both checks generate SOC 2 evidence as a side effect — log every run with an outcome row.

---

## 9. Deeper answers — second research pass

Working through the Section 5 open questions. Subsections labeled with the question number being addressed.

### 9.1 Billing Credits edge cases — Q-new-B / C / E

The three answers that decide whether the Stripe-only-with-Billing-Credits path is viable.

**Q-new-B: 100-grant ceiling (CONFIRMED, not blocking).**
The cap is **100 *unused* grants per Customer**, not 100 ever. A grant is "unused" if not voided, not expired, and either not yet effective or still has a positive balance. Drained or expired grants drop out of the count; new ones can be added. Sales math:

- Solo Pro: 1 grant/month for monthly quota + occasional pack purchases. Quota grant expires at period end (drops out). Pack grants live 12 months. Steady-state ceiling: ~12 active grants.
- Large Team buying 5 packs/month: 1 quota + ~60 active pack grants at the 12-month mark. Comfortable headroom.
- Stress case: a firm doing weekly bulk pack purchases would hit the cap around year 2 of buying ~2 packs/week. Mitigation: consolidate into larger packs, or merge expiring grants programmatically.

Worth alerting at 80% of the ceiling; not worth re-architecting around. ([Stripe — Billing credits](https://docs.stripe.com/billing/subscriptions/usage-based/billing-credits))

**Q-new-C: Refund of pack purchase — NOT automatic.**
Refunding the original Stripe payment **does not** auto-expire the corresponding Credit Grant. We must call the Credit Grant **Expire endpoint** ourselves. This is the kind of gap that produces silent revenue-recognition bugs if we don't know about it.

The (different) scenario Stripe *does* handle automatically: voiding an invoice that had credits applied **reinstates** the applied balance back into the original grant. That's about un-doing a draft invoice's credit application, not about refunding a paid pack purchase.

Refund flow we'll need to write:

```
On charge.refunded webhook:
  1. Look up the credit_ledger row for the original purchase
  2. Determine: is the pack fully unused, partially consumed, or fully consumed?
  3. Append a refund row to credit_ledger with delta zeroing the unconsumed portion
  4. Call Stripe Credit Grants Expire on the corresponding grant
  5. Optional: file a credit note for accounting if revenue was recognized
```

The "partially consumed pack got refunded" case is the gnarliest — it's the kind of policy decision we should write down explicitly (do we refund prorata, deny the refund, or eat the consumed portion?). ([Stripe — Billing credits](https://docs.stripe.com/billing/subscriptions/usage-based/billing-credits))

**Q-new-E: Monthly quota grants are NOT auto-renewed.**
Stripe doesn't have a "give this customer 25 free credits every month" config. We have to call the Credit Grants API ourselves on each renewal. This is one of the bits of orchestration that Stripe-only-with-Credits requires us to write. The hook point is `invoice.created` (or `customer.subscription.updated` for plan changes); on each fire we:

```
1. Look up the org's plan
2. If included quota grant for the new period doesn't exist:
     - Create it with expires_at = period_end
     - amount = plan.included_arcs
     - priority = lowest (drains last after older packs)
3. Persist the grant ID in our credit_ledger as a positive-delta row
   tagged reason='monthly_quota_grant'
```

Modest but real engineering. Means our renewal-handler is itself a system with retry/idempotency requirements — exactly the kind of thing Lago/Orb does for free.

### 9.2 Refund vs Credit Note vs Customer Balance vs Billing Credits — Q4

Four different concepts, often conflated. Decision rules for our model:

| Mechanism                  | What it is                                       | When we use it                                                                                                              |
|----------------------------|--------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------|
| **Refund**                 | Money returned to the original card              | The 7-day refund window. Money actually leaves us.                                                                           |
| **Credit Note**            | A formal *invoice adjustment* document           | When a paid invoice was wrong (over-billed for ARCs we shouldn't have charged for). Generates a legal accounting document. Can include a refund, a customer-balance credit, or be marked "credited outside Stripe." |
| **Customer (Invoice) Balance** | A general credit attached to the Customer that reduces the *next* invoice | Goodwill credits, support adjustments, "we ate this overage" — anything not specifically about metered ARC use. Drains on next invoice automatically. |
| **Billing Credits / Credit Grants** | Credits specifically usable against *metered* line items | Our ARC packs and monthly quota. Doesn't pay the licensed flat fee. |

Notable: Reserve/Rollback means we never charge for a failed Review in the first place. So credit notes for "this Review didn't deliver" should be rare — Rollback handles it before billing. Credit notes are mostly the safety net for "we discovered after the invoice closed that this charge was wrong."

For Accordli specifically:
- 7-day satisfaction refund → **Refund API** + **Credit Grant Expire** for any pack involved.
- Mid-period plan downgrade refund of overpaid portion → **Credit Note** with `customer_balance` allocation (so it eats the next month's flat fee instead of paying out cash).
- Goodwill or apology credit → **Customer Balance** for $ off; or **Credit Grant** if we want to give them ARCs specifically.

([Stripe — Issue credit notes](https://docs.stripe.com/invoicing/dashboard/credit-notes), [Stripe — Customer credit balance](https://docs.stripe.com/invoicing/customer/balance))

### 9.3 Proration on plan changes — Q3 / Q-new-D

Stripe handles **dollar proration on licensed (flat) prices automatically**. For metered prices, **proration is not applied** — the meter-based items just keep accumulating against whatever Price is current at the time of metering.

Implications for us:

- Pro ($200) → Gold ($400) on day 15 of 30: Stripe credits ~$100 unused-Pro and charges ~$200 prorated-Gold. The licensed-fee math is automatic.
- The **included ARCs side is ours**. If Pro included 10 ARCs and the customer used 6, and Gold includes 25, what is the new included balance for the rest of the period? Policy decision; one defensible answer is "remaining_pro_arcs (4) + new_gold_minus_used (25-6=19) → 19 ARCs available for the remainder," another is "fresh 25 prorated by remaining time." We have to spec this and implement it via Credit Grant manipulation when the upgrade happens.
- Downgrades behave the inverse way and should *probably* take effect at the next period boundary, not immediately — solves the "user downgraded after using 24 of 25 ARCs and now has -22 ARCs" problem. Stripe supports scheduled subscription updates for exactly this case.

`proration_behavior` enum:
- `create_prorations` — default; generates proration line items but doesn't invoice them until period end (or a threshold)
- `none` — no proration, change applies clean
- `always_invoice` — issue an invoice immediately for the proration

Our likely default: **`create_prorations`** for upgrades (charge the difference at next regular invoice), **scheduled at period end** for downgrades. ([Stripe — Prorations](https://docs.stripe.com/billing/subscriptions/prorations))

### 9.4 Plan-change UX — Q-new-D

Confirmed: **Stripe Customer Portal will not let customers change a subscription that has metered items or multiple products.** Cancel works; switch-from-Pro-to-Gold does not. Every Accordli plan has both a licensed item and a metered item, so we will own the entire upgrade/downgrade flow in our own UI.

Probably the right shape:

```
Account → Plan & Billing
  ┌───────────────────────────────────────┐
  │ Current plan: Solo Pro                │
  │ Renews on: May 30, 2026                │
  │ ARCs this period: 6 of 10 used         │
  │ Pack credits: 7 (expires 2027-02-12)   │
  ├───────────────────────────────────────┤
  │ [ Switch to Gold ]  [ Cancel plan ]   │
  └───────────────────────────────────────┘
```

Where `[ Switch to Gold ]` opens our own confirmation dialog, calls the Subscription Update API (with `proration_behavior` we choose), creates the new monthly-quota Credit Grant for Gold, and reflects in the UI. `[ Cancel plan ]` can deep-link to the Customer Portal cancel flow if we want Stripe's pre-built cancellation reasons survey.

### 9.5 Smart Retries / dunning — Q9

Stripe Smart Retries is solid for *what it does*: ML-driven retry-time selection over a configurable window. Default is 8 retries / 14 days. After exhaustion, the subscription transitions to one of three terminal states we choose:
- `canceled` — service revoked
- `unpaid` — invoices stay draft, customer can pay manually later
- `past_due` — keeps retrying per a separate retry rule

Stripe sends emails at retry events if we enable it in the dashboard (configurable templates, basic). What it does NOT give us:
- **In-app banner / nudge UX.** We build that.
- **Grace-period semantics in the product.** Up to us whether `past_due` orgs can still run reviews. Best practice (per industry data) is 7–14 days of full access while retries continue, then progressive feature degradation, then suspension.
- **Re-instate-on-update.** When a customer adds a new card, retries should resume; we wire this on `customer.updated` / `payment_method.attached`.

Reasonable Accordli policy:
- 8 retries / 14 days (Stripe default)
- Days 1–7: full access; in-app banner + email
- Days 8–14: limit to viewing prior reviews + adding payment method; no new analyses
- Day 15: subscription canceled; org goes read-only
- Re-instatement: new payment method → manual retry of latest invoice → resume normal state

([Stripe — Smart Retries](https://docs.stripe.com/billing/revenue-recovery/smart-retries))

### 9.6 Webhook handler in Go — Q12

Pulled from Stripe's docs and stripe-go v82's API. The shape:

```go
import (
    stripe "github.com/stripe/stripe-go/v82"
    "github.com/stripe/stripe-go/v82/webhook"
)

func handler(w http.ResponseWriter, r *http.Request) {
    // 1. Read RAW body. Don't let any middleware re-marshal it.
    body, err := io.ReadAll(r.Body)
    if err != nil { http.Error(w, "", 400); return }

    // 2. Verify signature. ConstructEvent enforces 5-min timestamp tolerance.
    event, err := webhook.ConstructEvent(
        body,
        r.Header.Get("Stripe-Signature"),
        webhookSigningSecret, // env var, NOT the API key
    )
    if err != nil { http.Error(w, "", 400); return }

    // 3. Idempotent dedupe BEFORE doing work.
    inserted, err := db.InsertProcessedWebhook(ctx, event.ID, event.Type)
    if err != nil { http.Error(w, "", 500); return } // retry later
    if !inserted {
        // already processed — ack and exit
        w.WriteHeader(200); return
    }

    // 4. Enqueue a River job to process. Return 200 within milliseconds.
    if err := riverClient.Insert(ctx, &ProcessStripeEventArgs{
        EventID: event.ID,
        Payload: body,
    }, nil); err != nil {
        // Roll back the dedupe row so the next retry can pick it up
        db.DeleteProcessedWebhook(ctx, event.ID)
        http.Error(w, "", 500); return
    }

    w.WriteHeader(200)
}
```

Key invariants the doc nails down:

- **Use the raw body.** Re-marshaling the JSON breaks signature verification.
- **Stripe retries for up to 3 days with exponential backoff** if we return non-2xx or time out. So returning 5xx is a recoverable failure, not a data loss event. Plan for it.
- **No ordering guarantees.** We must not assume `customer.updated` arrives before `subscription.updated`, etc. Either: (a) re-query the API for current state inside the handler, or (b) make handlers commutative.
- **At-least-once delivery.** Dedupe on `event.id` is non-negotiable.
- **5-min timestamp tolerance** on signatures by default. Don't disable.
- **Async handling pattern.** Return 200 fast; do the actual work in a River job. Avoids 5xx-on-slow-handler that triggers spurious retries.
- **Idempotency keys on outbound calls.** When the handler then calls Stripe back (e.g., to create a Credit Grant), pass `Idempotency-Key = event.id + "-" + operation`. Protects against retry loops creating duplicate grants.

stripe-go v82 introduced a `stripe.Client` API in v82.1 that auto-adds idempotency keys to retry-prone requests; we can lean on that for outbound calls. ([Stripe — Webhooks](https://docs.stripe.com/webhooks), [stripe-go webhook package](https://pkg.go.dev/github.com/stripe/stripe-go/v82/webhook))

### 9.7 Team-plan seat enforcement — Q23

Stripe seat-quantity is just **billing math**: we set `quantity = 3` on the licensed item and it bills 3× the seat price. Stripe does **not enforce** that only 3 humans access the app — that's our entitlement layer.

Recommended shape:

- Internal source of truth: `org_users` table. Count of active members per org.
- On user invite accept (or SCIM provisioning event in future Enterprise tier): bump our internal count, and mirror to Stripe by updating the SubscriptionItem `quantity`.
- On user removal: same in reverse.
- The actual entitlement check ("is this user in this org and active?") is a local query — never asks Stripe.

For Accordli specifically, the Team plans' "3 seats" / "10 seats" are **fixed plan attributes**, not variable seat counts. Going from Small Team to a 5th seat means upgrading to Large Team or buying enterprise. So `quantity` stays at 1 on the Subscription (we're charging the flat plan fee, not per-seat). The 3-seat / 10-seat number is enforced *entirely in our app* and lives in the plans config, not in Stripe.

That's a simpler shape than the "Stripe per-seat quantity" pattern most SaaS uses. Worth being explicit about in our spec — it means we don't have the "atomic update of quantity" race condition that bites teams using Stripe seat-quantities. ([Stripe — Subscription quantities](https://docs.stripe.com/billing/subscriptions/quantities))

### 9.8 Stripe Quotes for Enterprise — Q24

The Stripe Quotes API is real and usable. A Quote is an object with line items, discounts, and an expiration; once accepted, it auto-creates an Invoice (one-time) or Subscription (recurring). Comes with PDF generation and a hosted accept URL. Reasonable for early enterprise deals (5–20) before we'd need a real CPQ tool.

Limitations to know:
- Bound by Stripe's pricing model. Custom enterprise terms ("net-30 with $5K one-time signing credit, 24-month commitment, 10% discount above 50 seats") map awkwardly. We'd encode them as multiple line items + a custom Subscription Schedule.
- No native MSA / contract-document generation. We pair the Quote with Docusign for the legal artifact.
- No native procurement-flow features (CC the buyer's accounts payable, vendor onboarding, etc.).

Minimum-viable enterprise flow: Docusign for the MSA + Stripe Quote for the commercial terms + manual subscription creation when the quote is signed. That works for the first 5–10 deals; beyond that the Orb deep-dive's "case for billing platform with hosted enterprise quoting" reasserts itself. ([Stripe — Quotes](https://docs.stripe.com/quotes))

### 9.9 Stripe Tax operating reality — Q5 confirmed details

Stripe Tax monitors economic-nexus thresholds *automatically* across US states based on ship-to addresses on our customers, and **alerts us by email + dashboard** when we cross a threshold for a state. The notification triggers at $10K/year in any location.

State thresholds vary widely:
- Most states: $100K in sales OR 200 transactions/year (some have removed the transaction count — Illinois did Jan 2026, others trending similarly).
- Higher: CA, NY, TX at $500K.
- 5 no-state-tax states: AK, DE, MT, NH, OR (some AK localities still impose tax).

Important: **Stripe Tax does not register us in the state.** When alerted, *we* file paperwork with that state's department of revenue (timeline: a few weeks per state). This becomes operational drag once we're in 10+ states.

For solo-lawyer customers in 25–30 states, we'll likely cross thresholds in 5–8 of them within year 1. Plan for the registration-paperwork operational load. ([Stripe — Monitor your obligations](https://docs.stripe.com/tax/monitoring))

### 9.10 Net-30 invoicing for Enterprise — supported, with caveats

Stripe Invoicing supports net-N terms cleanly. The mechanism is the **Subscription `collection_method` flag**:

| `collection_method`    | Behavior                                                               |
|------------------------|------------------------------------------------------------------------|
| `charge_automatically` | Default. Stripe auto-charges the saved card on the invoice date.        |
| `send_invoice`         | Stripe generates the invoice and emails a hosted-invoice URL. Customer pays via card / ACH / wire. Combined with `days_until_due=30` (or 60, 90) for net-N terms. |

When `send_invoice` is set, Stripe:
- Generates the invoice and emails it to the customer.
- Hosts a payable invoice URL (card / ACH / wire as enabled).
- Sends configurable reminder emails before and after the due date.
- Marks the invoice `open` → `past_due` → `uncollectible` based on age and our config.
- Lets us record out-of-band payments (e.g., mailed check) via the dashboard or API.

The Quotes API (§9.8) auto-creates `send_invoice` subscriptions when an enterprise quote is accepted — so the deal flow is: Quote → customer accepts → Subscription with net-30 → first invoice issued.

**Accordli mapping:**

- Solo Pro / Solo Gold / Small Team / Large Team: stay on `charge_automatically`. Self-serve sign-up means card-on-file, monthly-in-advance.
- **Enterprise: `send_invoice` with net-30 (or whatever the contract specifies).** B2B legal procurement will not accept autocharge for a signed-MSA contract. Net-30 is the table-stakes term; net-60 and net-90 occasionally come up.

**Practical caveats:**

1. **Net-30 means we extend credit.** If the customer doesn't pay, we eat it / send to collections / suspend service — and "suspend service" is operationally harder when there's a signed MSA. Standard B2B AR risk; Stripe doesn't insulate us. We should set internal policy on "how long past due before we suspend an enterprise customer" before the first deal.
2. **Service continues during the 30 days.** Entitlement layer must allow ARC consumption *before* payment lands. Different posture from autocharge customers — for net-30 customers, payment status is decoupled from access until a configurable past-due threshold.
3. **ACH settlement is 3–5 business days.** Money isn't instant; reconciliation cadence has to tolerate the lag.
4. **Dunning shape differs.** Autocharge: "card declined, retry" (Smart Retries handles). Invoice: "they haven't paid, escalate" (reminder emails, then human follow-up). Both flow through `invoice.payment_failed` / `invoice.marked_uncollectible` webhooks; we want **different in-app UX** for each — no point dunning an enterprise customer like a self-serve solo who just needs to update their card.
5. **Tax works the same.** Stripe Tax computes on the invoice regardless of collection method.

**Implication for the platform spec:** the Enterprise tier diverges from self-serve in more places than just "case-by-case pricing." Commercial terms, payment shape, entitlement-during-grace policy, and dunning UX all branch. Worth being explicit when we expand the Enterprise section of `accordli_platform_overview.md`. ([Stripe — Billing collection methods](https://docs.stripe.com/billing/collection-method))

---

## 10. Re-evaluating Stripe-only vs Lago in light of Section 9

The Orb deep-dive recommended Lago Cloud as a hedge against the credit-pack-ledger problem, written before Stripe Billing Credits existed. After this research round, the picture has shifted but is **not as decisively Stripe-only as Section 4 first suggested**.

### What got better for Stripe-only
- Credit pack expiry + priority-ordered consumption: native primitive.
- Refund infrastructure: well understood, well documented.
- Tax: handled.
- Dunning: handled (with our own UX layer).
- Webhook patterns: well-trodden ground.

### What got worse / clearer
- **Auto-grant on renewal is on us** to orchestrate. Stripe does not re-grant monthly quota. That's a small renewal-cycle handler we have to write, test, and reconcile.
- **Refund-of-pack semantics are on us.** Stripe doesn't auto-expire the grant — we have to call Expire ourselves. The "partial pack consumed then refunded" case needs explicit policy.
- **Plan-change UX is on us.** Customer Portal is no help. We implement the upgrade flow + the included-ARC math when plans change.
- **100-grant ceiling** isn't a blocker but is real. Needs monitoring.

### What stayed the same regardless of choice
- Our `usage_events`, `credit_ledger`, `reservations`, `billing_periods` tables.
- Two-phase Reserve/Commit/Rollback wrapper.
- Real-time entitlement enforcement.
- Reconciliation cron.
- Webhook handlers (just more of them with Stripe-only — we'd be the renewal orchestrator too).

### Updated decision matrix

| Path | Lines of orchestration code we own | Vendor risk | "We get billed wrong" risk | $ cost |
|---|---|---|---|---|
| **Stripe-only with Billing Credits** | More than I initially estimated. Renewal grant orchestration + plan-change handler + refund-grant-expiry logic + entitlement layer. Maybe ~2,500 lines. | Lowest (everyone uses Stripe) | Medium-low — Credit Grant + reconciliation goes a long way | Just Stripe processing fees |
| **Lago Cloud free + Stripe** | Less. Credit pack and renewal orchestration is Lago's. We own the entitlement layer + a smaller webhook handler set. ~1,500 lines. | Medium (Lago is younger; OSS hedge available) | Lower — Lago is purpose-built and does the credit-pack accounting | Stripe processing fees + $0 (Lago free up to ~$1M ARR) |
| **Orb startup program** | Same as Lago. | High (vendor lock; no OSS escape) | Lower — Orb is the most polished | $0 if accepted, ~$720+/mo if not |
| **OpenMeter + Stripe** | Mid. We own credit pack ledger; OpenMeter handles event ingestion. ~2,000 lines. | Low | Medium | ~$0–$200/mo |

### Honest read

The Stripe-only path is now **plausible** but the orb-deepdive's note that "the credit pack ledger is the single subsystem to be most paranoid about" still applies — we just shifted *most* of the paranoia to renewal orchestration and refund handling on top of Billing Credits, instead of building the ledger from scratch.

**Current lean (subject to team discussion):** Stripe-only with Billing Credits *is now a real default* for the MVP, and the Orb deep-dive's "Lago Cloud free as primary recommendation" is no longer the obvious right answer. The choice now resembles:

- **Stripe-only** if we want minimum vendor surface and are comfortable owning the renewal/refund orchestration.
- **Lago Cloud free** if we want the credit-pack subsystem (and renewal grants, and refund handling) to be someone else's job, and accept Lago as a vendor.

The decision probably hinges on how much of the renewal/refund orchestration we'd write ourselves anyway *because of our app's specific semantics* (e.g., the ARC Consumption rules in `Reviewer-v2.md` — sub-90% completion not chargeable, retries free — which Lago doesn't know about either; we're writing those rules in Go regardless).

Suggest: **default to Stripe-only for the prototype window**, write the renewal grant + refund expiry handlers, then re-evaluate at the end of Phase 1 (month 3) before paid launch. Migration cost from Stripe-only to Lago is moderate; we don't lose the ledger we built, we just demote Stripe Credit Grants to "ignore, Lago is now authoritative."

---

## 11. Updated open questions (after second pass)

Resolved this pass:
- ~~Q3, Q-new-D~~ — proration handled (§9.3, §9.4). Plan-change UX is ours.
- ~~Q4~~ — refund/credit-note/balance/credits decision matrix (§9.2).
- ~~Q9~~ — dunning shape (§9.5).
- ~~Q12~~ — webhook idempotency Go pattern (§9.6).
- ~~Q23~~ — seat enforcement model (§9.7). Accordli's plans are fixed-seat, simpler than per-seat-quantity.
- ~~Q24~~ — Stripe Quotes is fine for 5–10 enterprise deals (§9.8).
- ~~Q-new-A~~ — Stripe-only viable but not strictly better than Lago (§10).
- ~~Q-new-B/C/E~~ — Billing Credits edge cases mapped (§9.1).

Still open:
- **Q8** — full self-serve credit pack flow as a sequence diagram. Mostly answered piecemeal but worth a single artifact.
- **Q11** — migration path from Stripe-only to Lago, if we ever do it. Not urgent.
- **Q13** — exact reconciliation cron specs (queries, frequency, alert thresholds).
- **Q14** — one Customer per Org. **Confirm**: yes, one Stripe Customer per Accordli Organization, mirrored on org creation, never per-User.
- **Q15** — when to collect tax ID. Suggest: at sign-up checkout for Team plans (most have EINs); deferred for Solo plans. Not load-bearing.
- **Q16** — multi-currency. Out of scope; design choice now is "USD only, accept international cards." Doesn't lock us in.
- **Q17/18** — data residency. Stripe stores in US by default. EU residency exists but is contracted enterprise. Document our position on the trust page.
- **Q19** — audit-evidence shape. Stripe provides a report API + Sigma queries. Worth a small spike during SOC 2 prep, not now.
- **Q20** — test/live mode hygiene. **Confirm**: separate accounts (one Stripe org for staging, one for prod) is the safer path; single account with mode toggling works but mixes audit logs. Recommend separate.
- **Q21** — refund window enforcement. Our app logic; not Stripe's job. Spec the eligibility check (≤7 days since billing period start AND ≤2 reviews completed AND not previously refunded for this email).
- **Q25/26** — 12-month roadmap pressure. Re-evaluate at month 3 (Phase 1 close) and month 6 (Phase 2 close).

---

## 12. Updated next steps

In rough priority order, post-second-pass:

1. **Pick the path** for the prototype window: Stripe-only-with-Billing-Credits OR Lago-Cloud-free. Both are now defensible; Stripe-only is the lower-vendor-surface default. Team decision.

   1. Vineel: I'm convinced that MVP (solo practioners) + Phase 1 (Teams) should be Stripe-only (no lagos or orb.)

2. **Spec the renewal-grant handler** (if Stripe-only): on `invoice.created`, ensure the org has a Credit Grant for the new period covering plan-included ARCs.

   1. Vineel: Yes, spec it

3. **Spec the refund-of-pack handler**: define our policy for partial-consumption refunds and write the Credit Grant Expire flow.

   1. Vineel: Yes, spec it

4. **Spec the plan-change handler**: included-ARC math when going Pro → Gold (or any direction); proration_behavior choice; downgrade-at-period-boundary scheduling.

   1. Vineel: Let's defer this for now. We can always handle this manually in Stripe via customer support.

5. **Sketch the webhook handler in Go** as a prototype: signature verification + dedupe + River-job dispatch. ~150 lines; smoke-tests the pattern.

   1. Vineel: You can use terse psuedocode. We don't need go level of detail yet.

6. **Get a written sales-tax classification opinion** — still standing; tax advisor work, not engineering.

   1. Vineel: Can you clearly state the opinion we need to ask for?

7. **Set up two Stripe accounts** (staging + prod) once we begin building, to avoid mixed audit logs from day one.

   1. Next week.

8. **Design the in-app plan-and-billing UX**: current plan card, ARC usage gauge, pack purchase CTA, upgrade flow, cancel flow (deep-link to portal). Likely deserves its own design doc.

   1. Defer

---

## 13. Renewal-grant handler — spec

**Purpose.** On every billing-period rollover for every active subscription, ensure the org has a Credit Grant on its Stripe Customer covering its plan's monthly included ARCs, and mirror that into our local `credit_ledger`. Without this handler, included ARCs don't exist in Stripe's view and customers get charged metered overage from ARC #1 of every month.

### 13.1 Trigger

`invoice.created` webhook is the right hook. Fires ~1 hour before period end (Stripe-controlled), well before the new period actually rolls over.

`invoice.upcoming` is too early (fires days ahead, may never finalize). `customer.subscription.updated` fires for too many unrelated reasons.

### 13.2 Logic (pseudocode)

```
on invoice.created webhook:
  invoice = event.data.object
  if invoice.subscription is null: return     # one-off invoice (e.g. pack purchase)
  if invoice.billing_reason != 'subscription_cycle': return
                                              # only fire on natural renewals,
                                              # not upgrades / corrections / first invoices

  org    = lookup_org_by_stripe_customer(invoice.customer)
  plan   = org.current_plan
  period = { start: invoice.period_start, end: invoice.period_end }

  if grant_exists_for(org, period): return    # idempotent

  grant = stripe.CreditGrants.create(
    customer        = invoice.customer,
    amount          = plan.included_arcs,
    category        = 'paid',                 # not 'promotional'
    priority        = 100,                    # lowest — drains last;
                                              # purchased packs default to 50,
                                              # so packs drain first
    expires_at      = period.end,             # quota expires at period end
    applicability_config = {
      scope: { price_type: 'metered',
               prices: [METERED_ARC_PRICE_ID] }
    },
    metadata        = { org_id: org.id,
                        period_id: period.id,
                        kind: 'monthly_quota' },
    idempotency_key = f"{org.id}:{period.id}:quota"
  )

  insert credit_ledger (
    org_id           = org.id,
    delta            = +plan.included_arcs,
    reason           = 'monthly_quota_grant',
    source           = 'stripe',
    source_id        = grant.id,
    expires_at       = period.end,
    idempotency_key  = f"{org.id}:{period.id}:quota"
  )
```

### 13.3 Properties to verify

- **Idempotent on webhook redelivery.** Stripe `idempotency_key` matches the local one; second delivery is a no-op on both sides.
- **Idempotent on plan changes.** A mid-period upgrade fires `customer.subscription.updated`, *not* `invoice.created` for cycle. The renewal grant for the existing period was already created at the prior renewal. The Pro→Gold delta-of-quota question is the deferred manual-runbook scenario (§15).
- **Quota grant priority is *lowest*.** Older expiring packs drain first (they expire sooner). The monthly quota expires at period end — wasting it on this month's quota draws over a pack that the customer paid extra for.
- **No grant created if subscription is canceled / unpaid.** `invoice.created` doesn't fire on canceled subs; this is implicit.

### 13.4 Failure modes

| Failure                                  | Detection                                                | Recovery                                                                  |
|-----------------------------------------|----------------------------------------------------------|---------------------------------------------------------------------------|
| Stripe API error during grant create    | Webhook returns 5xx; Stripe retries up to 3 days         | Eventually succeeds; reconciliation cron flags org as missing-grant       |
| Local DB write succeeds, Stripe call fails | Webhook returns 5xx                                      | Stripe retries; idempotency keys make second attempt safe                |
| Webhook missed entirely (Stripe outage) | Reconciliation cron diff (ledger vs Stripe Credit Grants) | Backfill grant; alert on >24h drift                                       |
| Quota grant created, customer downgrades next day | Manual support runbook (§15)                            | Expire over-granted portion; create new grant matching new plan          |

### 13.5 Edge cases not handled by this spec

- **First subscription period.** First invoice after sign-up has `billing_reason='subscription_create'`, not `'subscription_cycle'`. Add a separate case (or call Credit Grants API directly from the sign-up flow). Note for the sign-up implementation.
- **Plan changes** that should affect the in-progress quota grant. Handled manually via §15.
- **Trials** — not in current pricing spec.

---

## 14. Refund-of-pack handler — spec

**Purpose.** When a customer is refunded for a previously-paid credit pack, our `credit_ledger` and the corresponding Stripe Credit Grant must reflect that the credits are no longer valid. Stripe does *not* auto-expire the grant (§9.1); we handle this end-to-end.

### 14.1 Policy decision required first

The hard question: **what happens when a partially-consumed pack is refunded?** Three options:

| Option | Refund semantics                                                          | Customer experience                       | Notes                              |
|--------|----------------------------------------------------------------------------|-------------------------------------------|------------------------------------|
| A      | Refund only allowed if pack is **100% unused**                             | Hard "no" if even 1 ARC consumed          | Lowest abuse risk                  |
| B      | **Prorated refund**: customer gets back $(unused/total) × purchase price; consumed ARCs kept | "We'll refund what you didn't use"        | Math is straightforward             |
| C      | **Full refund regardless** of consumption; we eat the consumed value       | "Sorry, here's all your money back"       | Highest abuse vector                |

**Decision (confirmed by Vineel, 2026-04-30): Option A is the self-serve default; Option B is available only via support intervention with audit log. Option C reserved for case-by-case exceptions.**

This aligns with the existing 7-day refund policy (≤2 contracts analyzed). Option A is the rule that the self-serve refund button enforces; Option B is what support reaches for to be generous in a specific situation.

### 14.2 Trigger

`charge.refunded` webhook, filtered to charges that originated from a credit-pack purchase.

How to identify pack purchases:
- At purchase time we set `metadata.purpose='credit_pack'`, `metadata.org_id`, `metadata.pack_size` on the PaymentIntent.
- The `charge.refunded` event lets us read this metadata via the parent PaymentIntent.

(Subscription-invoice refunds — usually credit notes — are a different code path, out of scope here.)

### 14.3 Logic (pseudocode, Option A + B-via-support)

```
on charge.refunded webhook:
  charge = event.data.object
  pi     = stripe.PaymentIntents.retrieve(charge.payment_intent)
  if pi.metadata.purpose != 'credit_pack': return

  org      = lookup_org_by_stripe_customer(charge.customer)
  pack_row = credit_ledger.find(source='stripe',
                                source_id=pi.id,
                                reason='pack_purchase')
  if pack_row is null:
    alert("refund for unknown pack purchase"); return

  consumed = sum(credit_ledger.delta where
                 reason='consumption' and
                 source_grant_id = pack_row.stripe_grant_id)   # see §14.4
  unused   = pack_row.delta - abs(consumed)

  # Option A check is enforced upstream (refund button disabled
  # if consumed > 0). If we reach this handler with consumed > 0,
  # support authorized an Option B refund. Either way: Stripe
  # says these dollars are gone, so the credits backing them
  # must be invalidated.

  # 1. Mirror to our ledger.
  insert credit_ledger (
    org_id           = org.id,
    delta            = -unused,
    reason           = 'refund',
    source           = 'stripe',
    source_id        = charge.id,
    idempotency_key  = f"refund:{charge.id}"
  )

  # 2. Expire the corresponding Stripe Credit Grant.
  stripe.CreditGrants.expire(
    pack_row.stripe_grant_id,
    idempotency_key=f"expire:{charge.id}"
  )

  # 3. Log for audit if Option B territory.
  if consumed > 0:
    audit_log("partial_refund_with_consumption",
              org=org.id, charge=charge.id,
              consumed=consumed, refunded=unused)
```

### 14.4 Pack attribution — the subtle bit

"Consumption was drawn from this pack" requires a rule: when a ReviewRun consumes 1 ARC and the org has multiple active Credit Grants, *which* grant did it draw from?

Stripe's priority-ordered consumption (lower priority drains first) is the source of truth. Our local ledger has to mirror that ordering when we record consumption rows.

**Implementation:** at Commit time, the consumption row records `source_grant_id` referencing whichever grant Stripe drew from. Discoverable by reading the invoice line items (or Credit Grants API) after Stripe applies them.

This means **the consumption row schema needs an extra column, `source_grant_id`.** Worth adding now, while we're speccing — used by both the refund handler and per-pack reporting.

### 14.5 Failure modes

| Failure                                            | Detection                       | Recovery                                                          |
|---------------------------------------------------|----------------------------------|-------------------------------------------------------------------|
| Stripe Credit Grant Expire fails                   | Webhook 5xx, Stripe retries     | Idempotency keys; eventually succeeds                             |
| Pack consumed beyond unused (race)                 | `unused` goes negative          | Alert; manual reconciliation; we either owe the customer ARCs back or the consumption was illegitimate |
| Partial refund issued without "support authorized" flag | Audit log review               | Operational policy issue, not a code bug                          |
| Ledger says pack drained, Stripe still has unexpired grant | Reconciliation cron diff       | Expire the grant; investigate why it wasn't expired earlier       |

### 14.6 Edge cases not handled

- **Refund of a subscription invoice** including metered overage. Different code path (credit notes); out of scope.
- **Chargebacks.** Different webhook (`charge.dispute.*`); we treat similarly to refunds but with a "disputed" status flag and may pre-emptively expire grants.

---

## 15. Manual plan-change support runbook

**Context.** Plan-change automation is deferred (§12 #4). Customers requesting plan changes go through support, which executes the change in the Stripe dashboard. This is the runbook.

### 15.1 Scope and authorization

- **Who can do this:** named support engineers (initially Vineel, Tom). Tracked in an internal access list.
- **Audit trail:** every plan change recorded in our admin tool with timestamp, customer, before/after plan, dollar impact, who executed.
- **In scope:** Pro↔Gold (Solo); Small↔Large Team; Solo→Team upgrades.
- **Out of scope:** Enterprise contract changes (separate workflow per §9.10 / §9.8).

### 15.2 Pre-change checklist

1. Confirm customer's request in writing (email is fine).
2. Pull current state: org_id, current plan, current period dates, ARC usage this period, active credit packs.
3. Decide effective date:
   - **Upgrade:** effective immediately; prorated charge for upgrade delta.
   - **Downgrade:** effective at next period boundary. (Avoids the "used 24 of 25 then downgraded to 10-ARC plan" gotcha.)
4. Decide ARC handling for the in-progress period (upgrades only):
   - Default: customer keeps unused ARCs from old plan; we add a `manual_top_up` Credit Grant for the new plan's ARC delta, prorated by remaining time in period.
   - Alternative: zero out old quota, grant full new quota for the remainder.
   - Communicate the choice explicitly to the customer.

### 15.3 Steps in Stripe dashboard

1. **Customer → Subscriptions** → open the active subscription.
2. **Update subscription** → change the licensed-fee SubscriptionItem to the new plan's Price ID.
3. **Proration:** select **"Prorate"** for upgrades, **"Don't prorate, schedule at next period"** for downgrades.
4. **Save.** Stripe will:
   - Upgrade: issue a proration invoice immediately (or roll into next invoice, per settings).
   - Downgrade: schedule the change for the next period anchor.
5. **Adjust the monthly quota Credit Grant** (upgrades only):
   - Find existing `monthly_quota` grant on the customer (Credits section).
   - Either (a) leave it; add new `manual_top_up` grant for the ARC delta prorated by remaining time, or (b) expire the old grant and create a new one for the full new quota for the remaining period.
   - Set `expires_at = period_end`. Priority 100 (matches auto-renewal grant).
6. **Update the local plan record** in our admin tool: set `org.current_plan_id` to the new plan version. **This is what entitlement reads — easy to forget.**

### 15.4 Post-change

- Email confirmation: new plan, effective date, dollar impact, new ARC quota, when it resets.
- Verify in Stripe that the next renewal will hit the new plan's Price.
- Verify in local DB that the renewal-grant handler (§13) will pick up the right plan at next cycle.
- Log the change in admin audit trail.

### 15.5 Common pitfalls

- **Forgetting step 6** (the local DB update). Stripe-side is correct but our entitlement service still thinks they're on the old plan. Customer hits unexpected limits.
- **Letting a downgrade apply immediately.** Always schedule downgrades at period boundary.
- **Trying to expire still-valid packs on plan upgrade.** Don't — packs stay valid for 12 months across plan changes.

### 15.6 When this runbook stops being enough

- Volume hits ~5 plan changes/month → automate Pro↔Gold first (highest-volume case).
- Repeated errors → tighten or automate.
- Enterprise customers with custom terms → their own deal-by-deal flow, not this runbook.

---

## 16. Plan versioning approach (lightweight, Stripe-only)

**Context.** With Lago/Orb off the table, we own plan versioning. The orb-deepdive correctly flagged this as a long-term risk: `if customer.plan_v == 1 { ... } else if v == 2 { ... }` everywhere is a poison pattern. Avoidable cheaply by being deliberate from day one.

### 16.1 Principle

**Never branch on plan version in code.** Plan attributes (monthly fee, included ARCs, seat count, feature flags) are *data*, looked up at runtime from a `plans` table. Code asks "what does this org's current plan allow?" and the answer comes from a row.

### 16.2 Stripe side

- **One Stripe Product per plan** (`Solo Pro`, `Solo Gold`, `Small Team`, `Large Team`). Stable across versions.
- **Multiple Stripe Prices per Product**, one per pricing version.
- **When pricing changes:** archive old Price (Stripe lets you do this without breaking existing subscribers), create new Price, point new sign-ups at it.
- **Existing subscribers stay on their archived Price.** Stripe does not auto-migrate. Migration is an explicit operational decision.

### 16.3 Our side

```
plans
  id              uuid    PK
  plan_key        text             -- 'solo_pro', 'solo_gold', etc. (stable)
  version         int              -- 1, 2, 3, ...
  stripe_product  text             -- prod_... (stable per plan_key)
  stripe_price    text             -- price_... (per version)
  monthly_fee     int              -- cents
  included_arcs   int
  seat_limit      int
  features        jsonb            -- e.g. {team_dashboard: true, sso: false}
  effective_from  timestamp
  effective_until timestamp        -- null = currently sold version
  UNIQUE(plan_key, version)
```

```
organizations
  ...
  current_plan_id  uuid REFERENCES plans(id)
  ...
```

`org.current_plan_id` points at a specific (plan, version) row. Existing customers keep pointing at their version forever unless we explicitly migrate them. New customers get the latest non-archived version.

### 16.4 Code patterns

```
# Reading a customer's allowance — what code looks like everywhere
plan = db.plans.get(org.current_plan_id)
if usage_this_period >= plan.included_arcs:
    charge_overage()

# NOT this (poison pattern):
if org.plan_version == 1: included = 10
elif org.plan_version == 2: included = 12
elif org.plan_version == 3: included = 15
```

The poison pattern is only avoidable if we go data-driven from row 1. The `plans` table costs ~20 lines on day one; retrofitting after 50 occurrences of `if version == ...` costs days.

### 16.5 Pricing-change runbook (when it happens)

1. Insert a new `plans` row: same `plan_key`, version+1, new amounts, new Stripe Price ID (created in dashboard or via API first).
2. Set `effective_until` on the previous row to now.
3. Update sign-up code to default to the new version (one-line config pointing at "latest by plan_key").
4. Existing customers continue on old version; their `current_plan_id` doesn't change.
5. Optional: send existing customers a "we're updating prices" email and offer migration.

### 16.6 What this gets us for free

- Old invoices remain reproducible — their plan version still exists.
- A/B testing prices to a subset of new sign-ups is "10% of new orgs get version N+1."
- Grandfather promises ("you keep this rate") are honored automatically.
- A customer asks "what was my plan in March 2026?" — look up their `current_plan_id` history (audit log on org table).

### 16.7 What it doesn't solve

- Custom enterprise pricing (one-off Prices per customer). Goes in a separate `enterprise_terms` table, not `plans`.
- Mid-version feature flag rollouts. Use feature flags via PostHog, not plan versions.
- Cross-cutting changes affecting *all* customers (e.g., new abuse-prevention limit). Code-level constants are fine.

---

## 17. Tax-advisor opinion request — drafted question

For when we engage a SaaS-experienced sales-tax advisor (suggested before $100K MRR, per §4.8). The question:

> **Subject: Sales-tax classification for AI-assisted contract-review SaaS sold to legal professionals**
>
> We are a B2B SaaS company (US-based, Delaware C-corp). Our product is a web application that uses AI models to analyze legal contracts uploaded by users and produces written findings, summaries, and reports. Customers are primarily attorneys and law firms (solo practitioners, in-house teams, and small-to-mid-size firms) located in all 50 states. Pricing is a recurring monthly subscription with included usage credits, plus optional one-time credit-pack purchases.
>
> We need a written taxability opinion covering:
>
> 1. **Classification.** For sales-tax purposes, is our service most properly characterized as: (a) prewritten/canned software (SaaS), (b) a digital good or digital automated service, (c) an information service, (d) a data-processing service, (e) a non-taxable professional or consulting service, or (f) something else? We are not providing legal advice and are not a law firm; the AI produces analytical outputs that the customer's attorney evaluates and uses.
>
> 2. **State-by-state taxability map.** For each US state (and DC), is our service taxable under the classification above? Please flag states where the classification is ambiguous (e.g., Texas's 80%-of-data-processing rule, or states that distinguish "delivered electronically" SaaS from professional services). For ambiguous states, indicate which classification we should adopt and why.
>
> 3. **Customer-type effects.** Does selling to a law firm vs. an in-house legal department vs. a solo practitioner change the taxability analysis in any state? Are there exemption certificates we should be collecting (resale, government, nonprofit), and at what point in the sign-up flow?
>
> 4. **Registration triggers.** What economic-nexus thresholds apply, and at what cumulative sales volume per state should we register? We use Stripe Tax for monitoring but not registration.
>
> 5. **Bundled-pricing treatment.** Our subscription bundles "AI compute" with "report generation" with "stored documents." Does the bundling affect taxability in states that treat components differently?
>
> Deliverable: a written memo we can rely on for compliance and that we can show to enterprise customers' tax teams during procurement. We expect to revisit annually.

The "AI-assisted but not legal advice" framing in (1) is the load-bearing distinction in some states between "taxable digital service" and "non-taxable professional service" — that's why it's worded carefully.

   


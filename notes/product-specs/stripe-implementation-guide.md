# Stripe Implementation Guide

This is the spec we implement against. All design decisions are made; this document captures *what we build*, not *why we picked it*. Rationale lives in `stripe-research-scratch.md`.

The Accordli MVP and Phase 1 are **Stripe-only**. No Lago, no Orb. Stripe handles subscriptions, invoicing, dunning, tax, and credit grants. Accordli owns real-time entitlement, the immutable accounting log, and reconciliation.

---

## 1. Glossary

### Stripe terms

- **Customer** — Stripe's record of a paying entity. Holds saved payment methods, address, tax ID, billing balance.
- **Product** — a stable plan identity (e.g., "Solo Pro"). Holds metadata; doesn't carry price.
- **Price** — one priced version of a Product. A Product has many Prices over its lifetime.
- **Subscription** — links a Customer to one or more Prices for recurring billing.
- **SubscriptionItem** — one line on a Subscription. A subscription with a flat fee and a metered overage has two items.
- **Invoice** — the legal billing document. Generated automatically from the Subscription; can be auto-charged or sent for manual payment.
- **Meter** — Stripe's event-ingestion primitive. Receives usage events (one per ARC committed), aggregates by Customer.
- **Credit Grant** — prepaid credits attached to a Customer that draw down on metered usage before card charges. Has `expires_at`, `priority`, and `applicability`. The required primitive for our pack model.
- **PaymentIntent / SetupIntent** — represent a single attempted charge or a card-save-for-later. Used during Checkout under the hood; rarely directly.
- **Webhook event** — every state change emits one. Delivered at-least-once, signed, ordered loosely.
- **Customer Portal** — Stripe-hosted page for cancel + payment-method updates.
- **Checkout** — Stripe-hosted/embedded payment surface for sign-up and one-time purchases.
- **Stripe Tax** — automatic sales-tax calculation per ship-to address.

### Accordli terms (in Stripe context)

- **Organization** — our top-level customer entity. Maps 1:1 to a Stripe Customer.
- **Subscription** — Stripe object; one per Organization.
- **ARC** (Agreement Review Credit) — our usage unit. Consumed by completed ReviewRuns.
- **Plan** — Solo Pro / Solo Gold / Small Team / Large Team / Enterprise. Each is a Stripe Product with at least one Price.
- **Pack** — a one-time purchase of 10 ARCs for $100, valid 12 months. Implemented as a one-off Stripe charge that creates a Credit Grant.
- **Reservation** — a hold on the org's ARC balance taken at the start of a ReviewRun. Lives in our DB only, not in Stripe.

---

## 2. Architecture

```
┌──────────────────────────────────────────────────────────┐
│ Accordli (Go API + worker)                               │
│  - usage_events       (append-only, source of truth)     │
│  - credit_ledger      (append-only; mirror of grants +   │
│                        all consumption)                  │
│  - reservations       (mutable; in-flight ARC holds)     │
│  - billing_periods    (per-org plan snapshot at period   │
│                        start)                            │
│  - plans              (versioned plan catalog)           │
│  - entitlement service (Reserve check =                  │
│       SUM(non-expired ledger) - SUM(active reservations))│
│  - Reserve / Commit / Rollback wrapper                   │
│  - webhook handler   (verify, dedupe on event.id,        │
│                        enqueue River job)                │
│  - reconciliation cron (ledger ↔ Stripe daily)           │
└──────┬─────────────────────────────────┬─────────────────┘
       │ outbound API                    │ inbound webhooks
       ▼                                 │
┌──────────────────────────────────────────────────────────┐
│ Stripe                                                   │
│  - Customer per Org                                      │
│  - Subscription per Org                                  │
│      • Item 1: licensed Price (monthly fee)              │
│      • Item 2: metered Price → Meter "arc_usage"         │
│  - Credit Grants (monthly quota + purchased packs)       │
│  - Stripe Tax (auto-applied)                             │
│  - Customer Portal (cancel + payment method only)        │
│  - Checkout (sign-up + pack purchase)                    │
└──────────────────────────────────────────────────────────┘
```

Stripe is the system of record for subscriptions, invoices, payments, and tax. Accordli is the system of record for usage events, credit balance (request-path), and audit. Reconciliation runs daily.

---

## 3. Plans in Stripe

Each plan is one Stripe Product with one currently-active Price. Pricing changes create new Price objects; old Prices are archived, never deleted.

| Plan         | Stripe Product       | Licensed Price | Included ARCs | Seat limit (app-enforced) |
|--------------|----------------------|----------------|---------------|----------------------------|
| Solo Pro     | `prod_solo_pro`      | $200/mo        | 10            | 1                          |
| Solo Gold    | `prod_solo_gold`     | $400/mo        | 25            | 1                          |
| Small Team   | `prod_small_team`    | $600/mo        | 40            | 3                          |
| Large Team   | `prod_large_team`    | $2,000/mo      | 130           | 10                         |
| Enterprise   | `prod_enterprise`    | per-deal Price | per-deal      | per-deal                   |

Every Subscription has exactly two SubscriptionItems:

1. **Licensed item** — the monthly fee, one-line flat charge.
2. **Metered item** — bound to a single Meter named `arc_usage`. Price is $10/ARC, no free tier (the included quota is delivered via Credit Grants, not a tiered Price).

The metered item's Price ID is the same across all plans (`price_metered_arc`). The plan distinction lives in (a) the licensed Price and (b) the size of the monthly Credit Grant.

The pack purchase is **not** a SubscriptionItem. It's a one-off charge that creates a Credit Grant.

---

## 4. Data model (Accordli side)

```
organizations
  id                   uuid PK
  stripe_customer_id   text UNIQUE NOT NULL
  current_plan_id      uuid REFERENCES plans(id)
  billing_status       enum   (active | past_due | suspended | canceled)
  ...

plans
  id                   uuid PK
  plan_key             text     -- 'solo_pro', 'solo_gold', etc.
  version              int
  stripe_product_id    text     -- stable per plan_key
  stripe_price_id      text     -- per version
  monthly_fee_cents    int
  included_arcs        int
  seat_limit           int
  features             jsonb
  effective_from       timestamp
  effective_until      timestamp  -- null = currently sold
  UNIQUE(plan_key, version)

billing_periods
  id                   uuid PK
  org_id               uuid
  plan_id              uuid REFERENCES plans  -- snapshot at period start
  period_start         timestamp
  period_end           timestamp
  stripe_invoice_id    text
  UNIQUE(org_id, period_start)

usage_events
  id                   uuid PK
  org_id               uuid
  user_id              uuid
  kind                 text     -- 'arc_consumed', etc.
  quantity             int
  resource_id          text     -- e.g. review_run_id
  billing_period_id    uuid
  idempotency_key      text UNIQUE
  metadata             jsonb
  created_at           timestamp

credit_ledger
  id                   uuid PK
  org_id               uuid
  delta                int      -- +N for grants, -1 per ARC consumed
  reason               enum   (pack_purchase | monthly_quota_grant |
                                consumption | refund | expiration |
                                adjustment)
  source               text     -- 'stripe' | 'review_run' | 'manual'
  source_id            text     -- payment_intent_id, review_run_id, etc.
  source_grant_id      text     -- on consumption rows: which Credit Grant
                                -- the ARC drew from. Null on others.
  expires_at           timestamp
  idempotency_key      text UNIQUE
  created_at           timestamp

reservations
  id                   uuid PK
  org_id               uuid
  review_run_id        uuid
  qty                  int      -- always 1 in current model
  status               enum   (reserved | committed | cancelled | expired)
  expires_at           timestamp  -- TTL, default now+30m
  created_at           timestamp

review_billing_outcomes
  review_id            uuid PK
  org_id               uuid
  outcome              enum   (pending | chargeable_committed |
                                free_platform_failure |
                                free_prefix_failure)
  source_review_run_id uuid
  reservation_id       uuid NULL
  consumption_ledger_id uuid NULL
  decided_at           timestamp NULL

processed_webhooks
  event_id             text PK   -- Stripe event.id; dedupe key
  event_type           text
  processed_at         timestamp
```

Append-only invariant: rows in `credit_ledger`, `usage_events`, `processed_webhooks` are never updated or deleted. `reservations` is mutable.

Balance read:

```sql
SELECT COALESCE(SUM(delta), 0)
FROM credit_ledger
WHERE org_id = $1
  AND (expires_at IS NULL OR expires_at > now());
```

Available-for-reservation:

```sql
balance - SUM(reservations.qty WHERE status = 'reserved')
```

Indexed on `(org_id, expires_at)` and `(org_id, status)` respectively.

---

## 5. Sign-up flow

1. User picks a plan on the marketing/app site.
2. App creates an Accordli Organization row with no `stripe_customer_id` yet.
3. App calls `stripe.Customers.create` with the Org's email and stores the returned `customer_id`.
4. App opens **Embedded Checkout** in subscription mode with:
   - The plan's licensed Price ID + the metered Price ID (both items).
   - `customer = customer_id`.
   - `mode = subscription`.
   - `automatic_tax = enabled`.
   - `consent_collection.terms_of_service = required`.
5. User completes Checkout. Stripe creates the Subscription, charges the first month, and saves the card.
6. App receives `checkout.session.completed` webhook. The handler:
   - Marks Organization `billing_status = active`.
   - Stores the Subscription ID.
   - Creates the **first** monthly quota Credit Grant (the renewal handler in §10 only fires on `subscription_cycle`, not the first invoice — so the first grant is created inline here).
   - Inserts the corresponding `+N` row into `credit_ledger`.
7. User is redirected back to the app and lands on the dashboard.

---

## 6. Subscription billing cycle

Every billing period (monthly, anchored to the sign-up date):

1. Stripe creates the next invoice (`invoice.created` webhook). At this point the renewal-grant handler fires (§10).
2. During the period, customer consumes ARCs. Each Commit pushes a meter event to Stripe (§8) and writes a `-1` row to `credit_ledger`.
3. At period end, Stripe finalizes the invoice. The licensed fee is charged. Any metered ARCs beyond the monthly quota Credit Grant draw down packs (priority 50) before charging the card; the licensed fee always charges.
4. Card is charged automatically (`charge_automatically` collection method).
5. `invoice.paid` webhook arrives; handler marks the period closed in `billing_periods`.

If charge fails: Smart Retries kicks off (§13).

---

## 7. ARC consumption — Reserve / Commit / Rollback

This is the core of the entitlement layer. Every ReviewRun is wrapped in this pattern.

### Reserve

Called when a ReviewRun is initiated, unless the Review already has a free platform-failure billing outcome.

If the Review has `review_billing_outcomes.outcome IN ('free_platform_failure', 'free_prefix_failure')`, Reserve is skipped. The retry is allowed to run without checking available ARCs because Accordli already failed to deliver a robust Review and is eating the recovery cost.

Otherwise, in one transaction at `REPEATABLE READ`:

```
balance     = SUM(credit_ledger.delta WHERE org=O AND non-expired)
held        = SUM(reservations.qty   WHERE org=O AND status='reserved')
available   = balance - held
if available < 1:
    return 402 with upgrade CTA
INSERT reservations (org=O, review_run=R, qty=1,
                     status='reserved',
                     expires_at = now() + 30m)
UPSERT review_billing_outcomes (
    review_id, org_id, outcome='pending',
    source_review_run_id=R,
    reservation_id=reservation.id)
return reservation_id
```

The 30-minute TTL bounds crashed-worker holds. A periodic sweep marks expired reservations as `status='expired'`.

### Commit

Called when the ReviewRun completes with ≥90% of lenses successful, unless the Review was already marked free because of a platform failure.

If `review_billing_outcomes.outcome IN ('free_platform_failure', 'free_prefix_failure')`, Commit does not write a `credit_ledger` consumption row and does not send a Stripe meter event. The retry results may complete the Review, but the Review remains free.

For normal chargeable Reviews:

```
BEGIN
  UPDATE reservations SET status='committed' WHERE id=R
  source_grant_id = stripe.determine_grant_for_consumption(...)
                    -- ask Stripe which grant the meter event will draw from;
                    -- in practice, mirror Stripe's priority-ordered selection
  ledger_row = INSERT credit_ledger (
      org=O, delta=-1, reason='consumption',
      source='review_run', source_id=R,
      source_grant_id = source_grant_id,
      idempotency_key=R)
  UPDATE review_billing_outcomes
     SET outcome='chargeable_committed',
         consumption_ledger_id = ledger_row.id,
         decided_at = now()
   WHERE review_id = ReviewID
COMMIT

stripe.Meters.createEvent(
    event_name = 'arc_usage',
    customer   = O.stripe_customer_id,
    value      = 1,
    timestamp  = now(),
    identifier = R)               -- idempotency on Stripe side
```

The Stripe meter event is the trigger for billing. Our local ledger is the trigger for entitlement.

### Rollback

Called when the ReviewRun ends in `partial` (<90%) or `failed`.

```
UPDATE reservations SET status='cancelled' WHERE id=R
-- no ledger row, no meter event: nothing happened
```

The Review is also marked with a free billing outcome:

```
if prefix failed:
    outcome = 'free_prefix_failure'
else:
    outcome = 'free_platform_failure'

UPSERT review_billing_outcomes (
    review_id, org_id, outcome, source_review_run_id,
    reservation_id, decided_at)
```

Once a Review has a free failure outcome, all user-initiated retries inside that same Review bypass Reserve and can never create a later consumption row or Stripe meter event.

### Outcome matrix

| Review outcome                                      | reservations row | review_billing_outcome | credit_ledger row | Stripe meter event |
|-----------------------------------------------------|------------------|------------------------|-------------------|--------------------|
| Initial run completed normally (≥90% lenses)         | committed        | chargeable_committed   | -1 consumption    | yes                |
| Initial run partial (<90%)                           | cancelled        | free_platform_failure  | none              | no                 |
| Initial run failed before prefix/lenses were usable   | cancelled        | free_prefix_failure    | none              | no                 |
| Retry after free platform/prefix failure completes    | none or cancelled | unchanged free outcome | none              | no                 |
| Crashed worker (TTL expired before outcome decided)   | expired (sweep)  | pending                | none              | no                 |

User-initiated retries within the same Review do not consume another ARC. If the original run already committed an ARC, retries reuse that charge. If the original run was marked free because Accordli failed to deliver a robust Review, retries bypass Reserve and remain free even if they eventually complete the Review.

---

## 8. Pack purchase

Self-serve "buy 10 ARCs for $100, valid 12 months."

1. User clicks "Buy 10 ARCs."
2. App opens Stripe Checkout in `mode=payment` (one-time) with:
   - Line item: $100, name "10 ARCs Credit Pack."
   - `customer = org.stripe_customer_id`.
   - `metadata.purpose = 'credit_pack'`, `metadata.org_id`, `metadata.pack_size = 10`.
3. User pays.
4. Webhook `checkout.session.completed` arrives (also `payment_intent.succeeded`). Handler keys off the first; ignores the second to avoid double-processing.
5. In one transaction:

```
INSERT credit_ledger (
    org_id          = org.id,
    delta           = +10,
    reason          = 'pack_purchase',
    source          = 'stripe',
    source_id       = payment_intent_id,
    expires_at      = now() + 365d,
    idempotency_key = event.id)
```

6. Outside the transaction, call:

```
grant = stripe.CreditGrants.create(
    customer        = org.stripe_customer_id,
    amount          = 10,
    category        = 'paid',
    priority        = 50,                       -- packs drain before quota
    expires_at      = now() + 365d,
    applicability_config = {
      scope: { price_type: 'metered',
               prices: [METERED_ARC_PRICE_ID] }
    },
    metadata        = { org_id: org.id,
                        pack_size: 10,
                        kind: 'pack' },
    idempotency_key = event.id)

UPDATE credit_ledger
   SET source_grant_id = grant.id
 WHERE idempotency_key = event.id
```

7. Return 200 to Stripe.

Failure between (5) and (6) is recovered by the reconciliation cron, which detects ledger rows missing a `source_grant_id` and creates the Stripe grant.

---

## 9. Refund handling

### Subscription refund (7-day money-back)

Conditions for self-serve eligibility (enforced in our app):

- ≤7 days since the current billing period started.
- ≤2 ReviewRuns have committed in the current period.
- Customer has not previously taken this refund (one-time per email).

If eligible, app calls `stripe.Refunds.create(charge=invoice.charge)` for the most recent invoice. Subscription is canceled at period end (or immediately, per customer choice). No special ledger handling — the period's monthly quota grant expires naturally at period end.

### Pack refund (Option A: 100% unused only, self-serve)

Conditions for self-serve eligibility:

- The pack's corresponding Credit Grant has 100% of its credits remaining (`consumed = 0`).
- ≤30 days since purchase (configurable).

Self-serve flow:
1. App verifies `consumed = 0` against the local ledger.
2. App calls `stripe.Refunds.create(payment_intent=pack.payment_intent_id)`.
3. `charge.refunded` webhook arrives.
4. Handler runs the pack-refund logic in §9.1 below.

### Pack refund (Option B: prorated, support-only)

Support tools allow refunding part of a pack with audit log. The same handler runs; it tolerates `consumed > 0` and emits an audit log row.

### 9.1 charge.refunded handler

```
on charge.refunded webhook:
  charge = event.data.object
  pi     = stripe.PaymentIntents.retrieve(charge.payment_intent)
  if pi.metadata.purpose != 'credit_pack': return    # subscription refund handled separately

  org      = lookup_org_by_stripe_customer(charge.customer)
  pack_row = credit_ledger.find(source='stripe',
                                source_id=pi.id,
                                reason='pack_purchase')
  if pack_row is null:
      alert("refund for unknown pack purchase"); return

  consumed = abs(SUM(credit_ledger.delta WHERE
                     reason='consumption' AND
                     source_grant_id = pack_row.source_grant_id))
  unused   = pack_row.delta - consumed

  INSERT credit_ledger (
      org_id          = org.id,
      delta           = -unused,
      reason          = 'refund',
      source          = 'stripe',
      source_id       = charge.id,
      idempotency_key = f"refund:{charge.id}")

  stripe.CreditGrants.expire(
      pack_row.source_grant_id,
      idempotency_key = f"expire:{charge.id}")

  if consumed > 0:
      audit_log("partial_refund_with_consumption",
                org=org.id, charge=charge.id,
                consumed=consumed, refunded=unused)
```

---

## 10. Renewal-grant handler

On every billing-period rollover, ensure the org has a Credit Grant covering its plan's monthly included ARCs.

### Trigger

`invoice.created` with `billing_reason = 'subscription_cycle'`. Other `billing_reason` values (e.g., `subscription_create`, `subscription_update`) are *not* the trigger — `subscription_create` is handled inline at sign-up (§5); upgrades are handled by the support runbook (§11).

### Logic

```
on invoice.created webhook:
  invoice = event.data.object
  if invoice.subscription is null: return
  if invoice.billing_reason != 'subscription_cycle': return

  org    = lookup_org_by_stripe_customer(invoice.customer)
  plan   = org.current_plan
  period = { id, start: invoice.period_start, end: invoice.period_end }

  if grant_exists_for(org, period): return

  grant = stripe.CreditGrants.create(
      customer        = invoice.customer,
      amount          = plan.included_arcs,
      category        = 'paid',
      priority        = 100,                      -- drains last (after packs)
      expires_at      = period.end,
      applicability_config = {
        scope: { price_type: 'metered',
                 prices: [METERED_ARC_PRICE_ID] }
      },
      metadata        = { org_id: org.id,
                          period_id: period.id,
                          kind: 'monthly_quota' },
      idempotency_key = f"{org.id}:{period.id}:quota")

  INSERT credit_ledger (
      org_id          = org.id,
      delta           = +plan.included_arcs,
      reason          = 'monthly_quota_grant',
      source          = 'stripe',
      source_id       = grant.id,
      source_grant_id = grant.id,
      expires_at      = period.end,
      idempotency_key = f"{org.id}:{period.id}:quota")
```

The monthly quota grant has lower priority than purchased packs so packs drain first. This protects the customer's pre-paid pack value from being wasted on the current month's free quota.

---

## 11. Plan changes (manual)

Plan changes are not automated in MVP. Customers requesting a change go through support.

The runbook lives at `notes/claude-code-artifacts/stripe-research-scratch.md` §15. Summary:

- Upgrades: effective immediately, prorate the licensed fee, add a top-up Credit Grant for the ARC delta prorated by remaining time.
- Downgrades: scheduled at next period boundary to avoid retroactive over-quota states.
- Always update the local `org.current_plan_id` after the Stripe-side change. This is the canonical step that's easy to forget.

---

## 12. Cancellation

Customer cancels via Stripe Customer Portal (deep-linked from our app). On `customer.subscription.updated` webhook with `cancel_at_period_end = true`:

- Mark `org.billing_status = canceled` (cancellation pending).
- Display "subscription ends on YYYY-MM-DD" in the app.

On `customer.subscription.deleted` webhook (period end reached):

- Mark `org.billing_status = canceled`.
- Set the org to read-only: existing data accessible, no new ReviewRuns.
- The monthly quota grant for the final period was scoped to expire at period end; it expires naturally.
- Any unused pack credits are forfeited (per the Refund Policy in `accordli_platform_overview.md`); the grants stay until their natural expiry but the org can't create ReviewRuns to consume them.

Re-instatement: customer signs up again under the same email → same Org reactivated, new Subscription created, new monthly quota grants flow normally. Old pack grants resume being usable.

---

## 13. Failed payment & dunning

Stripe Smart Retries handles the retry mechanics. Configuration in the Stripe dashboard:

- Number of retries: 8.
- Time window: 14 days.
- After exhaustion: `cancel`.

In our app:

- On `invoice.payment_failed`: mark `org.billing_status = past_due`. Show in-app banner + email notification. **Service continues for the first 7 days.**
- On day 8: limit access to existing data and payment-method updates only; no new ReviewRuns.
- On `invoice.marked_uncollectible` (typically day 15): subscription is canceled by Stripe; treat as cancellation per §12.
- On `customer.updated` or `payment_method.attached` while past_due: trigger a manual retry via `stripe.Invoices.pay(invoice_id)`. On success, the next webhook flips back to active.

---

## 14. Enterprise (net-30 invoicing)

Enterprise plans use `collection_method = send_invoice` with `days_until_due = 30` (or whatever the contract specifies).

Sign-up flow differs from self-serve:

- Sales creates a Stripe Quote referencing the per-deal Price.
- Customer accepts the quote (signs the MSA via Docusign separately).
- Subscription is created on quote acceptance with `collection_method = send_invoice`.
- Stripe emails invoices instead of auto-charging. Customer pays via card / ACH / wire from the hosted-invoice URL.

Operationally:

- Service continues during the 30-day window. Entitlement is decoupled from payment status until past-due grace expires.
- Past-due policy: configurable per contract; default 30 days past due before suspension review.
- Dunning UX is escalation emails + named account-manager outreach, not in-app banners.
- ACH settlement adds 3–5 business days; reconciliation tolerates the lag.

---

## 15. Webhook handling

### Endpoint

One endpoint: `POST /webhooks/stripe`. All events go through it.

### Handler shape

```
func handler(req):
  body = read raw body                       # do not parse before verifying
  event = stripe.webhook.ConstructEvent(
            body, req.headers["Stripe-Signature"], WEBHOOK_SECRET)
  if signature invalid: return 400

  inserted = INSERT processed_webhooks (event_id=event.id, event_type=event.type)
                     ON CONFLICT DO NOTHING
  if not inserted: return 200                -- already processed

  enqueue River job (event_id, raw_body)
  return 200                                  -- within milliseconds
```

The actual event handling runs in a River job, not the HTTP handler. The HTTP handler's only jobs are: verify signature, dedupe, enqueue, return 200.

### Events we handle

| Event                              | Handler action                                                            |
|-----------------------------------|----------------------------------------------------------------------------|
| `checkout.session.completed`       | Sign-up completion (§5) or pack purchase (§8), based on session metadata. |
| `invoice.created`                  | Renewal-grant handler (§10), filtered on `billing_reason`.                |
| `invoice.paid`                     | Mark billing period closed.                                                |
| `invoice.payment_failed`           | Set `billing_status = past_due`; notify (§13).                            |
| `invoice.marked_uncollectible`     | Treat as cancellation (§13).                                              |
| `customer.subscription.updated`    | Sync subscription state (cancel pending, status changes).                 |
| `customer.subscription.deleted`    | Cancellation finalization (§12).                                          |
| `charge.refunded`                  | Pack refund (§9.1) or no-op for subscription refunds.                     |
| `customer.updated`                 | Mirror billing-relevant fields; trigger retry if past_due.                |
| `payment_method.attached`          | Trigger invoice retry if past_due.                                        |

All other event types: log and ignore.

### Properties

- **Idempotent on event.id.** `processed_webhooks` table is the dedupe store.
- **Async processing.** River jobs handle work; HTTP handler returns 200 fast.
- **No ordering assumptions.** Handlers either re-query Stripe for current state or are commutative.
- **Outbound idempotency keys** on every Stripe API call, derived from `event.id` plus operation name.
- **5-minute timestamp tolerance** on signatures (Stripe default). Do not disable.

---

## 16. Reconciliation cron

Daily job. For each active org:

1. **Internal consistency.** Re-fold `credit_ledger` from genesis. Compare to a cached `current_balance` view (if maintained). Page on disagreement.
2. **External consistency.** Sum non-expired Credit Grants on the Stripe Customer. Compare to the local ledger sum of grant + consumption rows. Tolerate small lag (Stripe is eventually consistent on grant updates); page on persistent drift exceeding a threshold (e.g., >24h, >2 ARCs).
3. **Completeness checks.**
   - Every active subscription has a current-period monthly quota grant.
   - Every `pack_purchase` ledger row has a non-null `source_grant_id`.
   - No `consumption` ledger row has a null `source_grant_id`.
4. **Operational checks.**
   - Reservations stuck in `reserved` status beyond their TTL.
   - Webhooks marked processed but with no resulting ledger or state changes.

Each run logs an outcome row to a `reconciliation_runs` table for SOC 2 evidence.

---

## 17. Tax

Stripe Tax is enabled at the account level. Every Customer collects address information at sign-up; Stripe Tax computes and adds tax to every invoice automatically.

Our responsibilities:

- Collect address (and tax ID for B2B) at sign-up via Checkout's built-in fields.
- Monitor Stripe Tax's economic-nexus alerts (email + dashboard). When alerted, register with the relevant state's revenue department within the legal window.
- Maintain a written taxability opinion from a sales-tax advisor (see `stripe-research-scratch.md` §17 for the question to ask). Revisit annually.

We do **not**:

- Manage tax rates manually.
- Compute tax in our app.
- Register with states preemptively (only on alert).

---

## 18. Plan versioning

Pricing changes are inevitable. The plan-versioning model avoids version-conditional code branches entirely.

### Pattern

- One Stripe Product per plan, stable across versions.
- Multiple Stripe Prices per Product, one per pricing version.
- Old Prices are archived (not deleted) when superseded.
- Existing subscribers stay on archived Prices unless explicitly migrated.
- Local `plans` table mirrors this with `(plan_key, version)` rows. Each Org's `current_plan_id` points at a specific row.

### Code rule

Plan attributes are *data*, not code. Always read from `plans`:

```
plan = db.plans.get(org.current_plan_id)
limit = plan.included_arcs
```

Never:

```
if org.plan_version == 1: limit = 10
elif org.plan_version == 2: limit = 12
```

### When pricing changes

1. Create a new Stripe Price under the existing Product.
2. Insert a `plans` row with version+1 and the new Price ID.
3. Set `effective_until` on the previous row to `now()`.
4. Update sign-up code to default to the latest version per `plan_key`.
5. Existing subscribers remain on the old version. Migration is opt-in and customer-communicated.

---

## 19. Test / live mode

Two separate Stripe accounts:

- **Staging.** Used by all non-production environments (local dev, CI, staging deploy).
- **Production.** Used only by prod.

Each has its own API keys, webhook secrets, customers, products, prices, and event streams. They are completely isolated; no objects or events cross between them.

Local development uses the **Stripe CLI** to forward staging webhooks to localhost: `stripe listen --forward-to localhost:8080/webhooks/stripe`.

Production webhook secrets are stored in Azure Key Vault. Staging secrets are in Key Vault as well, separate path.

---

## 20. Monitoring & alerts

The observability surfaces that need to exist before launch:

| Signal                                              | Source                  | Threshold to alert                               |
|----------------------------------------------------|-------------------------|--------------------------------------------------|
| Webhook handler error rate                          | App logs                | >1% over 5 min                                   |
| Webhook signature verification failures             | App logs                | Any (potential attack indicator)                 |
| `processed_webhooks` insert conflicts (duplicates)  | DB metric               | Tracked, not alerted (informational)             |
| River job failures on Stripe-event jobs             | River dashboard         | >3 in an hour                                    |
| Credit-grant ceiling approaching                    | Reconciliation cron     | Any org at >80 active grants                     |
| Reconciliation drift (ledger vs Stripe grants)      | Reconciliation cron     | Any persistent drift >24h                        |
| Reservations stuck in `reserved` past TTL           | Reservation sweep       | Any unswept TTL-expired reservation              |
| `past_due` orgs                                     | DB query                | Tracked on dashboard; no auto-alert              |
| Failed Stripe API calls                             | App logs                | Rate threshold; investigate spikes               |
| Stripe Tax nexus alerts                             | Stripe email + dashboard | Manual triage; track in ops queue               |

Surface the operational signals on a single internal dashboard (Grafana or PostHog).

---

## 21. Implementation phases

Rough order of build, smallest unit first.

### Phase 0 — Skeleton

- Two Stripe accounts (staging, prod).
- One Product per plan, current Price for each.
- Single Meter `arc_usage`.
- Webhook endpoint with signature verification + dedupe + River job dispatch (no event handling yet).
- Stripe CLI wired to local dev.

### Phase 1 — Sign-up and consumption

- Embedded Checkout sign-up flow.
- `checkout.session.completed` handler creates first monthly quota grant.
- `plans` table populated.
- Reserve / Commit / Rollback wrapper around ReviewRun.
- Meter event on Commit.
- `credit_ledger` and `reservations` tables in use.

### Phase 2 — Renewals and packs

- Renewal-grant handler (§10).
- Pack purchase Checkout flow (§8).
- `charge.refunded` handler (§9.1).
- Self-serve subscription refund eligibility check.
- Customer Portal deep-link for cancel.

### Phase 3 — Operations

- Reconciliation cron.
- Monitoring dashboard with the alerts in §20.
- Manual plan-change runbook published; admin tool for the local-DB step.
- Stripe Tax enabled, address collection in Checkout.

### Phase 4 — Enterprise

- `send_invoice` collection method.
- Stripe Quote integration.
- Past-due policy configurable per org.
- Account-manager dunning workflow.

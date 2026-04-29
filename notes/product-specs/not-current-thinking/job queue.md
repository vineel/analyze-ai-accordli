# Tool Queue

# Overview
Accordli Analyze has several needs for a queue mechanism.

# Tools
## Document Tool Runs
* Export of HTML, PDF, and DOCX reports and memos

## LLM Tool Runs
* A prompt that is run against cloud-based LLM models. Will usually use Claude models (sonnet and haiku) hosted by Anthropic or Azure.
* Result Ingestion of the prompt output, which is usually a JSON structure.

## NOT Tool Runs
Originally these tasks were thought to be queue-based jobs. However, it looks like they are fast enough to be done "inline" in the API.

* Ingest and conversion of DOCX contracts
* Ingest and conversion of supplementary documents
* Reserving a Review Credit is handled by the API caller, not the job.

# Multiple Run Structure
For this product, the most important sequence looks like this:

1. API enqueues 1 prompt to analyze the contract + supplemental docs (the "router prompt").
2. The Worker spins up a goroutine to process the job from the queue (the "jobfunc").
3. The jobfunc sets up the prompt to act as a "Prefix" for later prompts, and executes the LLM call.
4. While prompt is running, the browser FE polls the API to find the status of the job.
5. The LLM streams back results to the jobfunc. The jobfunc buffers the stream and parses it incrementally to update a "current message" and a counter in the db (e.g., "found 4 issues" → 5 → 10), and may emit permanent messages ("Skipped liability cap, which does not apply.").
6. When the job finishes, go code extracts metadata and persists it to the db.
7. The router output is analyzed and a 10-12 prompt subset of 15-20 analysis prompts is chosen for this agreement.
8. Each of those prompts is enqueued with the Prompt prefix.
9. The browser polls the API and shows per-lens spinners, partial counts, and per-lens results as each finishes.
10. As each prompt finishes, an ingestfunc parses the JSON response and writes to the db. Each prompt has its own ingestfunc, with shared helpers.
11. Failed prompts are retried. Vendor-class failures fail over to the second vendor (e.g., Azure Sonnet down → retry on Anthropic).

---

# Design Decisions

## Queue substrate: River (Postgres-backed)
- Same DB as job state, so "enqueue + state update" is one transaction.
- No second store of truth, no Redis ops burden.
- Workload is low-throughput / long-per-job (LLM calls), which fits Postgres-backed queues fine.

## Streaming output format: JSONL
- LLM emits one JSON object per line during streaming.
- Same parser handles streaming and final ingest.
- Avoids partial-JSON-parsing fragility.
- Supports both simple counts ("4 issues" → "5") and richer mid-stream events ("Skipped liability cap").
- An alternative considered was regex-counting tags in a partial JSON blob — rejected as too fragile once we want richer mid-stream signals.

## Vendor failover: per-prompt, eat the cache cost
- Each prompt failover is independent. If Azure errors on lens 4, retry lens 4 on Anthropic. Don't re-pin the whole review.
- This loses Anthropic's prompt cache benefit on the failed-over prompt (cache is provider-specific), but **UX trumps unit economics** — losing a customer to long waits is worse than the extra tokens.
- Track cache-hit-rate per review and per vendor as a metric — on a degraded-vendor day, COGS jumps and we want to see it, not discover it on the bill.
- Be deliberate about *what counts as a vendor reason* for failover:
  - Yes: 5xx, timeouts, rate-limit-without-retry-after, connection errors.
  - No: content-policy refusal, 4xx on bad input — these are deterministic. Retrying on the other vendor wastes money and gets the same answer.

## Coordinator pattern: state machine, not a long-lived job
The "router job → fan out 10-12 follow-ups → finalize" lifecycle is owned by a small state-machine package, not by a babysitter goroutine.

- `reviews` table tracks `state`, `router_job_id`, `prompts_total`, `prompts_completed`, `prompts_failed`.
- API enqueues the router job, sets `reviews.state = 'routing'`.
- Router jobfunc finishes → calls `ReviewSM.OnRouterComplete(review_id)`. That function reads the router output, picks the 10-12 prompts, enqueues each as a follow-up tagged with `review_id`, sets `prompts_total`, flips `state = 'analyzing'`. All in one txn.
- Each follow-up jobfunc finishes → calls `ReviewSM.OnPromptComplete(review_id, prompt_id)`. That runs ingestfunc, increments `prompts_completed`, and if `completed + failed == total`, flips `state = 'done'`.
- On permanent failure → `OnPromptFailed`, increments `prompts_failed`, same terminal-check logic.
- On user cancel → `OnCancel` flips `state = 'cancelled'`. Each follow-up jobfunc checks `state` before starting and bails early.

Mental model: **River is the transport, the state machine is the brain, jobfuncs are hands.**

Why not inline the dispatch logic in the router jobfunc:
- Retries: if dispatch crashes after the router LLM call succeeded, we can re-run dispatch against the persisted router output without re-running the (expensive) LLM call.
- Cancellation: one place to flip state.
- Testability: state machine is pure-ish, testable without spinning up a queue.
- Locality: review-lifecycle logic lives in one file, not scattered across jobfunc bodies.

## Per-prompt visibility in the UI
A user sees results lens-by-lens as they finish, not all-at-once at the end. Implication for the data model:

```
review_prompts (
  review_id,
  prompt_id,           -- "liability_lens", "ip_lens", etc.
  state,               -- pending | running | complete | failed
  vendor_used,
  partial_count,       -- updated while streaming, throttled (~500ms)
  current_message,     -- freeform status, also throttled
  started_at, completed_at,
  error
)
```

Per-lens issues live in their own tables (`issues`, `clauses`, etc.) keyed by `(review_id, prompt_id)`.

Flow per follow-up:
- jobfunc starts → `state = running`.
- During stream → throttled updates to `partial_count` / `current_message`. **Do not write on every chunk** — coalesce to ~500ms or every-N-events. This is a real DB-load concern with 12 concurrent prompts per review.
- On completion → ingestfunc writes rich data into `issues`/etc., then flips `state = complete`. Single txn so the FE never sees `complete` without the data behind it.
- On failure (after retries) → `state = failed`, error recorded. Other lenses keep going.

## Partial reviews are a valid end state
If 11 of 12 lenses succeed and 1 fails permanently, ship the review with 11 lenses' worth of results and a "this lens failed" indicator. We've already paid for the 11 LLM calls; failing the whole review is bad UX and bad economics.

Implications:
- "Review done" condition is `prompts_completed + prompts_failed == prompts_total`, not `prompts_completed == prompts_total`.
- Credit-refund rule needs to define the threshold (e.g., full credit consumed if ≥8 of 12 lenses succeeded; refund otherwise). **Open question — see below.**

---

# Open Questions

These came up but we didn't decide:

1. **Cancellation + credit-refund semantics.**
   - User closes browser mid-review — do remaining prompts keep running? (Probably no — `OnCancel` and per-jobfunc state check.) But what defines "closed"? FE explicit cancel button vs. just stopped polling? A poll-timeout heuristic is gross but maybe necessary.
   - On permanent multi-lens failure, when do we refund the credit? Suggested rule: full credit if ≥X of 12 lenses succeeded, refund otherwise. X = 8?
   - On user cancel before the router finishes, refund full credit.
   - On user cancel after fan-out, charge based on how many lenses had completed.

2. **Document export jobs.**
   - User-triggered ("Export PDF" button) and feel-synchronous? Or deferred?
   - Same River queue or separate? Suggestion: same River, separate worker pool — different latency/resource profile (CPU-bound, fast, deterministic) shouldn't share a worker pool with slow IO-bound LLM jobs.

3. **Concurrency caps.**
   - Per-vendor RPM/TPM ceilings (Anthropic and Azure both have org-level limits).
   - Per-org concurrency cap so one big customer can't starve others.
   - Worker-side cap on total in-flight LLM calls.
   - Probably fine to defer these to v1.1, but the queue + state machine should be designed so they can be added without restructure.

4. **Idempotency of ingestfunc.**
   - If a prompt's LLM call succeeds but ingestfunc crashes mid-write, retry will re-run the LLM call (waste) and may re-write (duplicate rows).
   - Two complementary fixes:
     - Persist raw LLM output to db before invoking ingestfunc → retry skips the LLM call.
     - Make each ingestfunc transactional and keyed on `(review_id, prompt_id)` so re-runs are idempotent no-ops.
   - Probably want both.

5. **Streaming-update throttling exact policy.**
   - 500ms? Every N parsed JSONL lines? Combination?
   - Need to balance "feels live" against DB write load (12 lenses × N reviews concurrently).

6. **What does "vendor reason" mean precisely.**
   - Sketched above (5xx/timeout/rate-limit yes; 4xx/policy no), but worth a real list before we ship.

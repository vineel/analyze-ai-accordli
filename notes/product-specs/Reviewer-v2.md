# Reviewer (Analysis Subsystem)

Reviewer is the subsystem that reviews agreements and produces findings.

---

## Core Concepts

### Matter
The top-level container for one agreement. A Matter holds the contract, supplemental documents, user-provided answers, and generated metadata. Once a Review has been run against a Matter, the Matter is locked — no new documents or agreements can be uploaded.

### Review
A Review is the user-facing object that represents one analysis of a Matter. It is a collection of Findings produced by running a set of Lenses. There will be multiple review types (e.g. Quick Review, Full Review, Risk Review).

The Review is the read model: it is what the web UX, reports, and user interactions work against. It does not own process state — that belongs to ReviewRun.

### ReviewRun
A ReviewRun is the process object that manages the async execution of a Review. It is a state machine that runs on the queue. A Review may have multiple ReviewRuns over its lifetime (e.g. initial run, user-initiated retries of failed lenses).

### Lens
A Lens is a prompt that examines the Matter documents from a specific angle and returns a set of Findings. Lenses are defined as Go templates stored in the git repo and uploaded to the database for runtime use.

### Finding
A Finding is one discrete issue or observation produced by a Lens. Findings have two parts:
- **Stable fields**: structured, indexable data (e.g. severity, category, lens reference)
- **Details**: a JSONB blob for lens-specific content that varies by lens type


---

## Run Process

### Step 1 — Build the Prefix
One prompt assembles the review system prompt, the contract, supplemental documents, and Matter metadata. The full text of this prompt is stored in the `prefix` field of the ReviewRun row. This is the context that all Lenses share.

The prefix uses Anthropic's prompt cache mechanism (`cache_control` blocks) so that parallel Lens calls benefit from cache hits against the same prefix content.

> Note: the prefix cache is vendor-specific. If the run fails over to a different vendor, the cache benefit is lost but the stored prefix text is still valid — it will be sent uncached to the new vendor.

### Step 2 — Run Lenses in Parallel
All Lenses for the review type are dispatched as parallel River jobs. Each job:
1. Reads the prefix from the ReviewRun row
2. Sends the prefix + lens prompt to the LLM
3. Buffers the full JSONL response
4. On success: parses the JSONL and persists all Findings for that Lens, then marks the LensRun completed
5. On failure: marks the LensRun failed (see Failure & Retry)

Lens output is never partially persisted. A Lens either completes and all its Findings are written, or it fails and nothing is written.

### Step 3 — UX Updates
The FE polls the Review at the lens level. Each Lens is shown as a spinner in the UI (up to 12 at once). As each LensRun completes, its spinner resolves and displays the finding count (e.g. "4 issues"). Spinners resolve independently as the parallel lenses finish.

---

## State Model

### ReviewRun States
`pending → running → completed | partial | failed`

- `completed`: all LensRuns succeeded
- `partial`: some LensRuns failed; user can retry individual lenses
- `failed`: the prefix step failed and the run could not proceed

### LensRun States
`pending → running → completed | failed`

Each LensRun record tracks:
- Status
- Retry count
- Active vendor (A or B)
- Finding count (populated on completion)

---

## Failure & Retry

### Vendors
- Vendor A: Azure
- Vendor B: Anthropic

### Prefix Step Failure
- Try up to 2 times on Vendor A
- If still failing, switch the entire run to Vendor B and retry
- If Vendor B also fails, mark the ReviewRun `failed`

### Lens Failure
- Try up to 2 times on Vendor A
- If still failing, switch that individual Lens to Vendor B and retry
- If Vendor B also fails, mark the LensRun `failed`

### Vendor vs. Non-Vendor Errors
- Specific errors that indicate a vendor is having trouble trigger an immediate vendor switch
- Errors unrelated to the vendor are retried up to N times on the current vendor before failing

### User-Initiated Retry
When a ReviewRun ends in `partial` state, the UX shows a Retry option for each failed Lens. Retrying a Lens:
- Creates a new River job within the same ReviewRun
- Reads the existing prefix from the ReviewRun row
- Follows the same vendor failover logic as the initial run

Retrying a failed lens does not create a new ReviewRun.

---

## Technical Notes

- **Queue**: River
- **Lens output format**: JSONL, buffered in memory until the lens completes
- **Finding storage**: stable fields as columns, variable content as JSONB
- **Prefix storage**: TEXT/JSONB field on the ReviewRun row (may be several hundred KB for large contracts)
- **Prompts**: Go templates in the git repo, uploaded to the database for runtime
- **Flexibility**: JSONB is preferred over rigid schemas while the product is evolving
- **Matter lock**: Once a ReviewRun has been initiated against a Matter, the Matter is locked

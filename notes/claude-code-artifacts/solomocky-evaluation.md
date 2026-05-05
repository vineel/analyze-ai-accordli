# Evaluation — `mocky-self-contained.md` (SoloMocky)

## Headline

SoloMocky inverts the central premise of `starter-app.md`. That document's whole argument is **the Scaffolding is the asset; Mocky is the throwaway stub that lives inside it.** SoloMocky proposes the opposite: build the throwaway stub first, then fold the Scaffolding in around it one piece at a time. That is a coherent path, but it's not a smaller version of the starter-app.md plan — it's a different plan, and the cost of going this way deserves to be named explicitly before either of us starts typing.

The right framing is: **SoloMocky is a spike, not a Phase 0.** Spikes are useful. They build intuition, force the data shapes into the open, and produce a running thing fast. But you don't add Scaffolding to a spike — you delete the spike and rebuild on Scaffolding. If we're going to call SoloMocky "the thing we add scaffolding to," we should expect the additions to look more like rewrites than retrofits.

## Feasibility

Buildable in a week or so by one engineer. None of the shortcuts are exotic. Local Postgres + a single Go binary serving the API and the Vite-built static assets + a few Anthropic calls in goroutines + one hardcoded user is well-trodden ground. No technology risk.

The risk is conceptual, not technical: which parts of SoloMocky **shape future Scaffolding work** versus **get thrown away**. The spec doesn't say. That's the gap to close before writing code.

## What we got wrong (or didn't say)

**1. Contradiction with `starter-app.md` on document conversion.** SoloMocky says "we will use our actual docx2md-go package." starter-app.md §6.5 says "pandoc baked into the worker container, Go post-processing." These can't both be current. Pick one and update the loser. (My read: docx2md-go is the right call if it already exists and is good — it's our package, no external binary, no container-image concerns; the starter-app.md line is probably stale.)

**2. The data model is unspecified.** Spec says "database rows are created to hold the review results." That single sentence is the load-bearing detail. Two paths:

- **Path A (cheap shortcut):** invent a SoloMocky-shaped schema like `lens_results { matter_id, lens_key, body_text }` and migrate later. This is the wrong call. The eventual `findings` table is a stable shape (see starter-app.md §5: narrow indexable fields + JSONB details). Migrating from a stub schema to that shape is non-trivial database surgery you'd want to avoid.
- **Path B (almost-free):** use the §5 schema verbatim — `matters / documents / review_runs / lens_runs / findings` — even with goroutines and hardcoded auth in front of it. It costs nothing extra to write the right CREATE TABLE statements on Day 1. **This is the recommendation.** The SoloMocky shortcuts should be in the *runtime* (no River, no billing, no WorkOS), not the *data model*.

The spec should make this explicit either way.

**3. `organization_id` strategy is implicit.** Hardcoded user/team/org in SoloMocky is fine. But every row in the eventual schema carries `organization_id`. If you skip writing it on the SoloMocky rows because there's only one Org, you'll backfill it later. Trivial cost to write it from the start with the constant Org UUID. Free correctness; do it.

**4. Matter lock invariant is missing.** Reviewer says: once a ReviewRun has run against a Matter, the Matter is locked. SoloMocky's flow describes one Run per Matter but doesn't say what happens if the user navigates back. State that Matters are locked after `Create New Matter` → run, same as Reviewer. Otherwise SoloMocky's UI will accidentally permit a behavior we then have to take away.

**5. UX regression on the spinner.** SoloMocky shows a single spinner with a wall-clock timer ("2m 49s"). starter-app.md and Reviewer both describe **per-Lens spinners that resolve independently into counts** ("12 facts", "6 questions"). The per-Lens UX is a load-bearing product feature — it's the whole reason a user tolerates the wait. Building the single-spinner version teaches us nothing about the per-Lens version, and we'll throw away the timer code wholesale. Cheap to do per-Lens from the start: even with sequential goroutine calls, each Lens completion writes a row, and the FE polls and renders whatever's done. Recommend per-Lens spinners in SoloMocky.

**6. Prompts as "seed data."** starter-app.md §6.6 says Lenses are Go templates in `/prompts/lens/`, hydrated at runtime, version recorded as `lens_runs.lens_template_sha`. Treating prompts as DB seed data is a different model — it implies a `prompts` table, a write path, and a divergence from git versioning. Pick the file-based path; it's already what Reviewer-v2 assumes.

**7. The serving model is half-stated.** "FE will use a vite proxy to hit the API" describes dev only. In SoloMocky's "serve FE from the same Go service," the prod-shape path is presumably `go embed` of the Vite build output. Worth saying so explicitly so it doesn't drift.

## Problems that will hurt sooner

These bite during the "add scaffolding one piece at a time" phase, i.e. weeks 2–8 of post-SoloMocky work.

**Goroutines → River is a rewrite, not a refactor.** River jobs are crash-resumable, retryable, durable. Goroutines aren't. The state machine you write for goroutine orchestration ("call A, await, call B, await, write rows, mark done") looks nothing like the River-shaped state machine ("enqueue lens jobs, each job reads its prefix, each job writes its findings, completion handler aggregates"). When you swap River in, every orchestration site changes. Mitigation: make SoloMocky's goroutine handler look like a River job — a function that takes a `job_id` and writes its own row to completion. Then "swap to River" is a thin dispatcher change rather than re-architecting.

**No Reserve / Commit / Rollback shape.** Even with billing deferred, the orchestration should be wrapped in a reservation-shaped scope: `reserve()` returns an ID, the run completes, `commit(id)` or `rollback(id)`. If you no-op the reservation calls in SoloMocky, you have the seam ready for Phase 6 of starter-app.md. If you don't, you'll thread the seam through the runtime later, which is exactly the kind of structural change that's painful after the fact.

**No `llm.Client` interface.** If SoloMocky does `anthropic.Call(...)` directly in handlers, you wrap it later. Same fix as above: write the interface from Day 1 with one implementation. Free; the shape is the point.

**No `organization_id` scoping.** Every query becomes `WHERE organization_id = $1`. If you skip the predicate on the assumption "there's only one org," you'll add it everywhere when WorkOS lands. RLS later will be an even bigger lift if there's a single global query layer to retrofit.

## Problems that will hurt later

These bite when Analyze replaces Mocky.

**Anthropic-direct-only defers the Foundry/ZDR story.** The defensible-to-lawyers security posture (per CLAUDE.md) leans on "Claude via Azure AI Foundry, ZDR configured." That's not just a vendor swap — it's auth (managed identity), networking (VNet integration possibly), and quota model differences. Doing Anthropic-direct first means the Foundry path is unproven for longer than it needs to be, and you don't get the vendor-A → vendor-B failover ladder until then either, which is one of Reviewer-v2's load-bearing reliability claims. If the goal is to validate the contract-analysis loop, Anthropic-direct is fine; just register that the *production* LLM path is unproven by SoloMocky.

**Sequential Lens calls mean no parallel cache-hit demonstration.** Anthropic prompt caching with `cache_control: ephemeral` does survive ~5 minutes between calls, so sequential calls *can* still hit the cache. But the timing test that proves it (call N+1 returns substantially faster than call N) is harder to make crisp. starter-app.md §6.6 calls out "verify the prompt cache hit in Helicone on Day 1" because it's one of the cheapest, highest-confidence tests of the LLM integration. Sequential SoloMocky still lets you observe `cache_read_input_tokens` in the response, so it's not lost — just less obvious.

**Single-tenant local PG with no RLS shape.** When RLS lands, you'll need to think about app role vs admin role, `app.current_org`, the bypass role for migrations, and the policies on every table. None of that is exercised by SoloMocky. The Postgres setup work in Phase 0 of starter-app.md is non-trivial and gets pushed to "later" by SoloMocky. Acceptable, but name it.

## What we'll regret

**The framing.** "SoloMocky → add scaffolding one piece at a time" is the regrettable choice if it actually means deferring the work in starter-app.md's Phase 0–2 (Bicep + Flex with CMK + WorkOS + Container Apps). Those phases are tedious and unrewarding compared to "see Claude analyze a contract." The temptation to keep iterating on the SoloMocky surface and never get back to Scaffolding is real. The starter-app.md plan front-loads the unrewarding work for a reason.

**Recommendation:** call SoloMocky a **spike**, not a starting point. Build it (it's small), use it to surface the data-model and prompt-shape questions, then **delete it** and start starter-app.md Phase 0. The data model and Lens prompts you produced in SoloMocky carry over; the goroutines, hardcoded auth, and local-PG harness don't.

**If we're keeping SoloMocky as the foundation we grow into Mocky-on-Scaffolding**, then the spec needs three additions:

1. Use the §5 schema verbatim. No shortcut schema.
2. Write the seams now: `llm.Client` interface, reservation-shaped wrapper, `organization_id` on every row, file-based prompts, per-Lens spinners.
3. State the order in which Scaffolding pieces go in (probably: WorkOS → River → Stripe → Foundry/Vendor B → RLS → CMK/Bicep), and acknowledge that each is a non-trivial cut-over, not a "drop-in."

## One specific to confirm

Is `docx2md-go` the current answer, replacing pandoc? If yes, update `starter-app.md` §6.5 the same day SoloMocky is approved — those two sentences will rot if they're left contradicting each other.

## TL;DR

Buildable. Cheap. The shortcuts are mostly fine. But the data model, the orchestration shape, and the per-Lens UX should not be shortcuts — they're free to get right on Day 1, and getting them wrong creates retrofit pain later. The bigger question is whether SoloMocky is a spike (delete it after) or a foundation (grow Scaffolding into it). Pick one and write that into the spec.

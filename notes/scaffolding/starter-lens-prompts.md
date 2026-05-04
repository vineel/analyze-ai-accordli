# Starter Lens Prompts

Two prompts to wire into the scaffolding starter app so the Reviewer-shaped fanout (Prefix + parallel Lenses → JSONL Findings) is exercised end-to-end on a generic .docx.

Structure mirrors the Reviewer-v2 spec:

- One **Prefix** is built per ReviewRun and stored on the ReviewRun row. It contains the system instruction, the document content (markdown from pandoc), and Matter metadata. The whole Prefix is wrapped in an Anthropic `cache_control` block so parallel Lens calls hit the cache.
- Each **Lens** is dispatched as its own River job. Each sends `Prefix + Lens suffix` to Claude and buffers the JSONL response.
- Each Lens emits **one Finding per JSONL line**. No preamble, no trailing prose. Stable fields are top-level keys; lens-specific extras live in `details`.

When the real Reviewer ships, these two starter Lenses are deleted and replaced by domain Lenses; the Prefix builder, the JSONL parser, the Findings persistence path, and the per-Lens spinner UI all stay.

---

## Shared Prefix (used by both Lenses)

This block is sent first on every Lens call and is the only block marked for caching. The Lens suffix that follows is small (~200 tokens) and uncached.

```
System:
You are an analyst reading a single business document on behalf of a careful
reader. Your job is to produce structured observations about this specific
document. You do not have access to any other source material. Do not invent
facts not present in the document. Do not editorialize. Quote the document
verbatim when asked for excerpts.

The document is supplied below as Markdown converted from the original file.
Headings, lists, tables, and inline emphasis are preserved; layout-only
formatting (page numbers, headers/footers, fonts) has been stripped.

You will receive one analysis instruction after this preamble. Follow only
that instruction. Output exactly the format requested — JSON Lines, one
JSON object per line, no surrounding prose, no Markdown fences, no trailing
commentary.

Matter metadata:
  matter_id:      {{ .MatterID }}
  filename:       {{ .OriginalFilename }}
  uploaded_at:    {{ .UploadedAt }}
  word_count:     {{ .WordCount }}

--- BEGIN DOCUMENT ---
{{ .DocumentMarkdown }}
--- END DOCUMENT ---
```

The `cache_control: { type: "ephemeral" }` marker is attached to this block
in the API call. The two Lens suffixes below are sent as a separate,
uncached block per request.

---

## Lens 1 — Entities & Key Facts

**Purpose.** Extraction lens. Pulls the indexable, factual surface area of the
document. Mostly deterministic; useful regardless of document type. Stands in
for a "what does this contract reference?" Lens in the eventual Reviewer.

**Lens suffix prompt:**

```
Instruction: Extract every notable entity and key fact from the document.

Emit one JSON object per line. No surrounding text. Schema:

{
  "kind":          "person" | "organization" | "date" | "money" |
                   "duration" | "location" | "defined_term" | "other",
  "value":         "<canonical form of the entity, ≤120 chars>",
  "excerpt":       "<verbatim quote from the document containing the entity, ≤200 chars>",
  "location_hint": "<nearest heading, section number, or 'preamble' / 'appendix' / null>",
  "details":       { ... lens-specific extras ... }
}

Rules:
- One JSON object per line. No arrays. No JSON wrapper. No code fences.
- The "excerpt" must appear in the document text verbatim. Do not paraphrase.
- "value" should be the canonical form (e.g. "Acme Corporation" not "Acme").
- For kind="defined_term", details = { "definition": "<the definition text>" }.
- For kind="money",        details = { "amount": <number>, "currency": "USD" | ... }.
- For kind="date",          details = { "iso": "YYYY-MM-DD" | null, "as_written": "<original>" }.
- For kind="duration",      details = { "iso8601": "P30D" | null, "as_written": "<original>" }.
- For kind="person",        details = { "role": "<role/title if stated>" | null }.
- For kind="organization",  details = { "role": "<party / counterparty / vendor / mentioned> " }.
- Skip routine boilerplate ("the parties", "this agreement").
- If the same entity is mentioned many times, emit it once, with the most informative excerpt.
- Cap output at 50 facts. If more exist, prefer those most central to the document's purpose.
- If the document contains no extractable facts, emit nothing.
```

**Stable Finding columns** (mapped from JSONL into the `findings` table):

| Column          | Source                                 |
|-----------------|----------------------------------------|
| `lens`          | `"entities_v1"` (constant per Lens)    |
| `kind`          | `kind` field                           |
| `value`         | `value` field                          |
| `excerpt`       | `excerpt` field (truncated to 200)     |
| `location_hint` | `location_hint` field                  |
| `details`       | `details` blob, stored as JSONB        |

**UI spinner resolves to:** "N facts" (count of rows for this LensRun).

---

## Lens 2 — Open Questions a Reader Would Ask

**Purpose.** Judgment lens. Surfaces ambiguities, contradictions, and notable
gaps actually present in the document. Closer in shape to a Reviewer Lens that
flags risks; the same plumbing will carry that traffic later.

**Lens suffix prompt:**

```
Instruction: Identify open questions a thoughtful reader would want answered
after reading this document. Focus on ambiguities, contradictions, undefined
terms, missing information, and things that are stated but unclear. Do not
invent issues; every question must be grounded in something the document
actually says (or notably fails to say).

Emit one JSON object per line. No surrounding text. Schema:

{
  "severity":        "low" | "medium" | "high",
  "category":        "ambiguity" | "contradiction" | "missing_info" |
                     "undefined_term" | "inconsistency" | "other",
  "question":        "<the question, phrased plainly, ≤200 chars>",
  "why_unclear":     "<short explanation of why this is unclear, ≤300 chars>",
  "related_excerpt": "<verbatim quote from the document, ≤200 chars, or null
                      for missing-info findings where there is nothing to quote>",
  "location_hint":   "<nearest heading or section reference, or null>",
  "details":         { ... lens-specific extras ... }
}

Severity guidance:
- high   — a reasonable reader cannot proceed without resolving this
- medium — the document is usable but the question materially affects interpretation
- low    — minor ambiguity, stylistic, or non-load-bearing

Rules:
- One JSON object per line. No arrays. No JSON wrapper. No code fences.
- "related_excerpt", when non-null, must appear in the document verbatim.
- For category="missing_info", set related_excerpt = null and use why_unclear
  to describe what would be expected and is absent.
- For category="undefined_term", details = { "term": "<the term>" }.
- For category="contradiction", details = { "other_excerpt": "<the second
  conflicting quote, verbatim>" }.
- Cap output at 25 questions. Prefer high/medium over low when capped.
- If the document has no notable open questions, emit nothing.
```

**Stable Finding columns:**

| Column          | Source                                  |
|-----------------|-----------------------------------------|
| `lens`          | `"open_questions_v1"` (constant)        |
| `severity`      | `severity` field                        |
| `category`      | `category` field                        |
| `question`      | `question` field                        |
| `excerpt`       | `related_excerpt` field (nullable)      |
| `location_hint` | `location_hint` field                   |
| `details`       | `why_unclear` + `details` blob, JSONB   |

**UI spinner resolves to:** "N questions" (count of rows for this LensRun).

---

## Implementation notes for the starter

- **Two River job kinds**: `lens.entities_v1` and `lens.open_questions_v1`.
  Both read `prefix` from the ReviewRun row and emit Findings into the same
  `findings` table; only the `lens` column distinguishes them.
- **One row per JSONL line, all-or-nothing.** Buffer the full response, parse
  every line, and only persist when every line parsed cleanly. A malformed
  line fails the LensRun (matches Reviewer-v2's "never partially persist").
- **Cache_control is on the Prefix block, not the Lens suffix.** Verify in
  Helicone (dev/staging only — see memory: prod is metadata-only) that the
  second Lens call shows a cache hit.
- **Vendor failover on Lens errors.** Both Lenses are single LLM calls and
  exercise the same A→B failover path Reviewer needs. Wire the failover on
  one and the other inherits it.
- **JSON schema lives in code.** Validate each parsed line against a Go
  struct before persisting; reject the LensRun if a row fails to validate.
  This is the contract that survives the swap from starter Lenses to real
  Lenses.
- **No ARC reservation in the starter.** Reserve/Commit/Rollback is a real
  subsystem; wire the entitlement plumbing as a separate scaffolding step
  rather than coupling it to LLM-call validation.

---

## Why these two, restated for the file

A useful starter Lens pair has to disagree under the same Prefix. Entities
extraction is deterministic and dense; Open Questions is judgmental and
sparse. Running them in parallel against the same .docx exercises:

- prompt-cache hit on the second Lens (same Prefix)
- divergent token volumes and latencies (spinners resolve at different times)
- the JSONL → Findings pipeline against two different schemas
- vendor failover on at least one Lens, since both can fail independently

When Reviewer ships, both files in `prompts/lens/` are replaced; everything
between the queue dispatch and the UI render survives the swap.

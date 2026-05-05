# Lens prompts

File-based Go templates, hydrated at runtime by `core/lens`.

Naming: `<lens_key>.tmpl` where `lens_key` matches `lens_runs.lens_key` (e.g. `entities_v1.tmpl`, `open_questions_v1.tmpl`).

Each Lens template emits the **Lens suffix** only — the shared Prefix (system prompt + Matter metadata + markdown) is assembled by the Prefix builder in `core/reviewrun` and prepended at call time.

Versioning: bump the `_vN` suffix when the prompt changes meaningfully. The git SHA of the template file at run time is recorded on `lens_runs.lens_template_sha`.

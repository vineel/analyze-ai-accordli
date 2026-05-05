-- 0004_run_summary.sql
--
-- ReviewRun-level summary text. The summary is one non-Lens LLM call
-- per Run that produces a short overview rendered above the per-Lens
-- panels (starter-app.md §6.7, open question #3 confirmed: keep it on
-- the Run row, not as a Finding).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE review_runs
    ADD COLUMN summary TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE review_runs
    DROP COLUMN summary;
-- +goose StatementEnd

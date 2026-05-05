-- 0003_runs.sql
--
-- Reviewer's hot-path tables per starter-app.md §5: review_runs, lens_runs,
-- findings.
--
-- organization_id is denormalized onto lens_runs and findings so that
-- future RLS policies stay single-table — the alternative (policies that
-- join up through review_runs → matters) was rejected as too expensive on
-- the hottest read path.
--
-- findings carries `category` as the only stable cross-Lens enum;
-- per-Lens shape (kind / severity / etc.) lives in `details` JSONB until
-- product evidence justifies promoting one. Adding a Lens later does not
-- require an ALTER TABLE.

-- +goose Up
-- +goose StatementBegin

CREATE TABLE review_runs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    matter_id           UUID NOT NULL REFERENCES matters(id) ON DELETE CASCADE,
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status              TEXT NOT NULL DEFAULT 'pending',
    prefix              TEXT,
    prefix_token_count  INTEGER,
    reservation_id      UUID,
    vendor              TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at        TIMESTAMPTZ,
    CHECK (status IN ('pending', 'running', 'completed', 'partial', 'failed')),
    CHECK (vendor IS NULL OR vendor IN ('A', 'B'))
);
CREATE INDEX idx_review_runs_matter ON review_runs(matter_id);
CREATE INDEX idx_review_runs_org_status ON review_runs(organization_id, status);

CREATE TABLE lens_runs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_run_id       UUID NOT NULL REFERENCES review_runs(id) ON DELETE CASCADE,
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    lens_key            TEXT NOT NULL,
    lens_template_sha   TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending',
    retry_count         INTEGER NOT NULL DEFAULT 0,
    vendor              TEXT,
    finding_count       INTEGER,
    error_kind          TEXT,
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    CHECK (vendor IS NULL OR vendor IN ('A', 'B'))
);
CREATE INDEX idx_lens_runs_review_run ON lens_runs(review_run_id);
CREATE INDEX idx_lens_runs_org_status ON lens_runs(organization_id, status);

CREATE TABLE findings (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_run_id       UUID NOT NULL REFERENCES review_runs(id) ON DELETE CASCADE,
    lens_run_id         UUID NOT NULL REFERENCES lens_runs(id) ON DELETE CASCADE,
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    lens_key            TEXT NOT NULL,
    category            TEXT,
    excerpt             TEXT,
    location_hint       TEXT,
    details             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (excerpt IS NULL OR length(excerpt) <= 200)
);
CREATE INDEX idx_findings_review_run ON findings(review_run_id);
CREATE INDEX idx_findings_lens_run ON findings(lens_run_id);
CREATE INDEX idx_findings_org_lens ON findings(organization_id, lens_key);
CREATE INDEX idx_findings_details_gin ON findings USING gin (details);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS findings;
DROP TABLE IF EXISTS lens_runs;
DROP TABLE IF EXISTS review_runs;
-- +goose StatementEnd

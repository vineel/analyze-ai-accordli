-- db/schema.sql
--
-- Single source of truth for the SoloMocky/Mocky schema while we are pre-prod.
-- Per CLAUDE.md and solomocky-to-mocky-plan.md: no migrations until first prod
-- customer (Phase 8). Schema changes go here, then `make reset` rebuilds.
--
-- Phase 0 collapsed the original goose migrations 0001-0004 into this file:
--   0001_init.sql        -> Identity (organizations, departments, users, memberships)
--   0002_matters.sql     -> Matters and documents
--   0003_runs.sql        -> Reviewer hot path (review_runs, lens_runs, findings)
--   0004_run_summary.sql -> review_runs.summary
--
-- RLS lands in Phase 2; until then `organization_id` is filtered at the API
-- layer in every repo method. The `accordli_app` / `accordli_admin` role split
-- also arrives in Phase 2.
--
-- gen_random_uuid() is built into Postgres core (>=13); no extension needed
-- yet. uuidv7() (per CLAUDE.md DB preferences) is queued for adoption when
-- we either upgrade to a release that ships it or add the pg_uuidv7 extension
-- on Azure Flex; for now gen_random_uuid() stays in place to keep the schema
-- portable.

-- ---------------------------------------------------------------------------
-- Identity
-- ---------------------------------------------------------------------------

CREATE TABLE organizations (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workos_org_id      TEXT UNIQUE,
    stripe_customer_id TEXT UNIQUE,
    name               TEXT NOT NULL,
    tier               TEXT NOT NULL DEFAULT 'solo',
    is_solo            BOOLEAN NOT NULL DEFAULT TRUE,
    billing_status     TEXT NOT NULL DEFAULT 'active',
    metadata           JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at         TIMESTAMPTZ
);

CREATE TABLE departments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    is_default      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_departments_org ON departments(organization_id);

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workos_user_id  TEXT UNIQUE,
    email           TEXT UNIQUE NOT NULL,
    current_dept_id UUID REFERENCES departments(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE TABLE memberships (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workos_membership_id TEXT UNIQUE,
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    department_id        UUID REFERENCES departments(id),
    role                 TEXT NOT NULL,
    status               TEXT NOT NULL DEFAULT 'active',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_memberships_user_org ON memberships(user_id, organization_id);
CREATE INDEX idx_memberships_org ON memberships(organization_id);

-- ---------------------------------------------------------------------------
-- Matters and documents
-- ---------------------------------------------------------------------------

CREATE TABLE matters (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id    UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    department_id      UUID NOT NULL REFERENCES departments(id),
    created_by_user_id UUID NOT NULL REFERENCES users(id),
    title              TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'draft',
    locked_at          TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at         TIMESTAMPTZ,
    CHECK (status IN ('draft', 'locked'))
);
CREATE INDEX idx_matters_org_dept ON matters(organization_id, department_id);
CREATE INDEX idx_matters_created_at ON matters(organization_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE documents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    matter_id   UUID NOT NULL REFERENCES matters(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    blob_url    TEXT,
    content_md  TEXT,
    filename    TEXT,
    size_bytes  BIGINT,
    sha256      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (kind IN ('original', 'markdown')),
    UNIQUE (matter_id, kind)
);
CREATE INDEX idx_documents_matter ON documents(matter_id);

-- ---------------------------------------------------------------------------
-- Reviewer hot path
-- ---------------------------------------------------------------------------

CREATE TABLE review_runs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    matter_id           UUID NOT NULL REFERENCES matters(id) ON DELETE CASCADE,
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status              TEXT NOT NULL DEFAULT 'pending',
    prefix              TEXT,
    prefix_token_count  INTEGER,
    reservation_id      UUID,
    vendor              TEXT,
    summary             TEXT,
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

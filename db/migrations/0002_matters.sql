-- 0002_matters.sql
--
-- Matters and their Documents per starter-app.md §5. A Matter is the
-- top-level container for one agreement; once a ReviewRun has been
-- initiated against it, it is locked (status='locked', locked_at set).
--
-- documents.kind is 'original' (the uploaded .docx) or 'markdown'
-- (the converted output of docx2md-go). One Matter has one of each at
-- the starter; multi-document Matters are deferred.

-- +goose Up
-- +goose StatementBegin

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
    -- Each kind appears at most once per Matter at the starter.
    UNIQUE (matter_id, kind)
);
CREATE INDEX idx_documents_matter ON documents(matter_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS matters;
-- +goose StatementEnd

-- 0001_init.sql
--
-- Identity tables per starter-app.md §5: organizations, departments,
-- users, memberships. Narrow stable shape; JSONB on organizations.metadata
-- for anything WorkOS/Stripe-specific that doesn't justify a column yet.
--
-- RLS NOT INSTALLED in Phase 0. Policies land in 0006_rls.sql once
-- enough handlers exist to validate them. The application-layer
-- `organization_id = $org` predicate is in place from Day 1; RLS is
-- belt-and-suspenders.
--
-- The deferred_at columns (`deleted_at`) implement soft delete; queries
-- default to `WHERE deleted_at IS NULL`. The 30-day sweep that hard-
-- deletes them lives in Phase 7.

-- +goose Up
-- +goose StatementBegin

-- gen_random_uuid() is built into Postgres core (≥13); no extension needed.

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

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS departments;
DROP TABLE IF EXISTS organizations;
-- +goose StatementEnd

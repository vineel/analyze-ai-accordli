package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Matter struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	DepartmentID    uuid.UUID
	CreatedByUserID uuid.UUID
	Title           string
	Status          string
	LockedAt        *time.Time
	CreatedAt       time.Time
}

type Matters struct{ pool *pgxpool.Pool }

func (r *Matters) Create(ctx context.Context, orgID, deptID, userID uuid.UUID, title string) (*Matter, error) {
	var m Matter
	err := r.pool.QueryRow(ctx, `
		INSERT INTO matters (organization_id, department_id, created_by_user_id, title)
		VALUES ($1, $2, $3, $4)
		RETURNING id, organization_id, department_id, created_by_user_id, title, status, locked_at, created_at
	`, orgID, deptID, userID, title).Scan(
		&m.ID, &m.OrganizationID, &m.DepartmentID, &m.CreatedByUserID,
		&m.Title, &m.Status, &m.LockedAt, &m.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Matters) Get(ctx context.Context, orgID, id uuid.UUID) (*Matter, error) {
	var m Matter
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, department_id, created_by_user_id, title, status, locked_at, created_at
		FROM matters
		WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
	`, orgID, id).Scan(
		&m.ID, &m.OrganizationID, &m.DepartmentID, &m.CreatedByUserID,
		&m.Title, &m.Status, &m.LockedAt, &m.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Matters) ListForOrg(ctx context.Context, orgID uuid.UUID) ([]*Matter, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, department_id, created_by_user_id, title, status, locked_at, created_at
		FROM matters
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Matter
	for rows.Next() {
		var m Matter
		if err := rows.Scan(
			&m.ID, &m.OrganizationID, &m.DepartmentID, &m.CreatedByUserID,
			&m.Title, &m.Status, &m.LockedAt, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// Lock flips status='draft' → 'locked' and stamps locked_at. Idempotent
// on already-locked rows (no-op).
func (r *Matters) Lock(ctx context.Context, orgID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE matters
		SET status = 'locked', locked_at = now()
		WHERE organization_id = $1 AND id = $2 AND status = 'draft'
	`, orgID, id)
	return err
}

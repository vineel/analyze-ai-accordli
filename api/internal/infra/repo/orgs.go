package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Orgs struct{ pool *pgxpool.Pool }

// EnsureMockyTrio idempotently creates the hardcoded Mocky Org, Dept,
// and User if they don't already exist. Used by the boot-time seed and
// by `make seed`.
func (r *Orgs) EnsureMockyTrio(
	ctx context.Context,
	orgID, deptID, userID uuid.UUID,
	orgName, deptName, userEmail string,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO organizations (id, name, tier, is_solo, billing_status)
		VALUES ($1, $2, 'solo', TRUE, 'active')
		ON CONFLICT (id) DO NOTHING`,
		orgID, orgName); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO departments (id, organization_id, name, is_default)
		VALUES ($1, $2, $3, TRUE)
		ON CONFLICT (id) DO NOTHING`,
		deptID, orgID, deptName); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, email, current_dept_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING`,
		userID, userEmail, deptID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO memberships (user_id, organization_id, department_id, role, status)
		VALUES ($1, $2, $3, 'owner', 'active')
		ON CONFLICT (user_id, organization_id) DO NOTHING`,
		userID, orgID, deptID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

var ErrNotFound = errors.New("not found")

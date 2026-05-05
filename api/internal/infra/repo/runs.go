package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReviewRun struct {
	ID               uuid.UUID
	MatterID         uuid.UUID
	OrganizationID   uuid.UUID
	Status           string
	Prefix           *string
	PrefixTokenCount *int
	ReservationID    *uuid.UUID
	Vendor           *string
	Summary          *string
	CreatedAt        time.Time
	CompletedAt      *time.Time
}

type ReviewRuns struct{ pool *pgxpool.Pool }

func (r *ReviewRuns) Create(ctx context.Context, matterID, orgID uuid.UUID) (*ReviewRun, error) {
	var rr ReviewRun
	err := r.pool.QueryRow(ctx, `
		INSERT INTO review_runs (matter_id, organization_id, status)
		VALUES ($1, $2, 'pending')
		RETURNING id, matter_id, organization_id, status, prefix, prefix_token_count, reservation_id, vendor, summary, created_at, completed_at
	`, matterID, orgID).Scan(
		&rr.ID, &rr.MatterID, &rr.OrganizationID, &rr.Status,
		&rr.Prefix, &rr.PrefixTokenCount, &rr.ReservationID, &rr.Vendor,
		&rr.Summary, &rr.CreatedAt, &rr.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &rr, nil
}

func (r *ReviewRuns) Get(ctx context.Context, orgID, id uuid.UUID) (*ReviewRun, error) {
	var rr ReviewRun
	err := r.pool.QueryRow(ctx, `
		SELECT id, matter_id, organization_id, status, prefix, prefix_token_count, reservation_id, vendor, summary, created_at, completed_at
		FROM review_runs
		WHERE organization_id = $1 AND id = $2
	`, orgID, id).Scan(
		&rr.ID, &rr.MatterID, &rr.OrganizationID, &rr.Status,
		&rr.Prefix, &rr.PrefixTokenCount, &rr.ReservationID, &rr.Vendor,
		&rr.Summary, &rr.CreatedAt, &rr.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &rr, err
}

func (r *ReviewRuns) GetByMatter(ctx context.Context, orgID, matterID uuid.UUID) (*ReviewRun, error) {
	var rr ReviewRun
	err := r.pool.QueryRow(ctx, `
		SELECT id, matter_id, organization_id, status, prefix, prefix_token_count, reservation_id, vendor, summary, created_at, completed_at
		FROM review_runs
		WHERE organization_id = $1 AND matter_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, orgID, matterID).Scan(
		&rr.ID, &rr.MatterID, &rr.OrganizationID, &rr.Status,
		&rr.Prefix, &rr.PrefixTokenCount, &rr.ReservationID, &rr.Vendor,
		&rr.Summary, &rr.CreatedAt, &rr.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &rr, err
}

func (r *ReviewRuns) SetPrefix(ctx context.Context, id uuid.UUID, prefix string, tokenCount int, vendor string, reservationID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE review_runs
		SET prefix = $1, prefix_token_count = $2, vendor = $3, reservation_id = $4, status = 'running'
		WHERE id = $5
	`, prefix, tokenCount, vendor, reservationID, id)
	return err
}

func (r *ReviewRuns) SetSummary(ctx context.Context, id uuid.UUID, summary string) error {
	_, err := r.pool.Exec(ctx, `UPDATE review_runs SET summary = $1 WHERE id = $2`, summary, id)
	return err
}

func (r *ReviewRuns) Finalize(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE review_runs
		SET status = $1, completed_at = now()
		WHERE id = $2
	`, status, id)
	return err
}

type LensRun struct {
	ID              uuid.UUID
	ReviewRunID     uuid.UUID
	OrganizationID  uuid.UUID
	LensKey         string
	LensTemplateSHA string
	Status          string
	RetryCount      int
	Vendor          *string
	FindingCount    *int
	ErrorKind       *string
	StartedAt       *time.Time
	CompletedAt     *time.Time
}

type LensRuns struct{ pool *pgxpool.Pool }

func (r *LensRuns) Create(ctx context.Context, reviewRunID, orgID uuid.UUID, lensKey, templateSHA string) (*LensRun, error) {
	var lr LensRun
	err := r.pool.QueryRow(ctx, `
		INSERT INTO lens_runs (review_run_id, organization_id, lens_key, lens_template_sha, status)
		VALUES ($1, $2, $3, $4, 'pending')
		RETURNING id, review_run_id, organization_id, lens_key, lens_template_sha, status, retry_count, vendor, finding_count, error_kind, started_at, completed_at
	`, reviewRunID, orgID, lensKey, templateSHA).Scan(
		&lr.ID, &lr.ReviewRunID, &lr.OrganizationID, &lr.LensKey, &lr.LensTemplateSHA,
		&lr.Status, &lr.RetryCount, &lr.Vendor, &lr.FindingCount, &lr.ErrorKind,
		&lr.StartedAt, &lr.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &lr, nil
}

func (r *LensRuns) MarkRunning(ctx context.Context, id uuid.UUID, vendor string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE lens_runs
		SET status = 'running', vendor = $1, started_at = COALESCE(started_at, now())
		WHERE id = $2
	`, vendor, id)
	return err
}

func (r *LensRuns) MarkCompleted(ctx context.Context, id uuid.UUID, findingCount int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE lens_runs
		SET status = 'completed', finding_count = $1, completed_at = now()
		WHERE id = $2
	`, findingCount, id)
	return err
}

func (r *LensRuns) MarkFailed(ctx context.Context, id uuid.UUID, errorKind string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE lens_runs
		SET status = 'failed', error_kind = $1, completed_at = now()
		WHERE id = $2
	`, errorKind, id)
	return err
}

func (r *LensRuns) ListByReviewRun(ctx context.Context, orgID, reviewRunID uuid.UUID) ([]*LensRun, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, review_run_id, organization_id, lens_key, lens_template_sha, status, retry_count, vendor, finding_count, error_kind, started_at, completed_at
		FROM lens_runs
		WHERE organization_id = $1 AND review_run_id = $2
		ORDER BY lens_key
	`, orgID, reviewRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*LensRun
	for rows.Next() {
		var lr LensRun
		if err := rows.Scan(
			&lr.ID, &lr.ReviewRunID, &lr.OrganizationID, &lr.LensKey, &lr.LensTemplateSHA,
			&lr.Status, &lr.RetryCount, &lr.Vendor, &lr.FindingCount, &lr.ErrorKind,
			&lr.StartedAt, &lr.CompletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &lr)
	}
	return out, rows.Err()
}

type Finding struct {
	ID             uuid.UUID
	ReviewRunID    uuid.UUID
	LensRunID      uuid.UUID
	OrganizationID uuid.UUID
	LensKey        string
	Category       *string
	Excerpt        *string
	LocationHint   *string
	Details        json.RawMessage
	CreatedAt      time.Time
}

type Findings struct{ pool *pgxpool.Pool }

// PersistAll writes the findings for one LensRun atomically. Either
// every row lands or none does — Reviewer requires "all-or-nothing"
// (Reviewer-v2 §"Step 2 — Run Lenses in Parallel").
func (r *Findings) PersistAll(ctx context.Context, fs []*Finding) error {
	if len(fs) == 0 {
		return nil
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, f := range fs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO findings
				(review_run_id, lens_run_id, organization_id, lens_key, category, excerpt, location_hint, details)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, f.ReviewRunID, f.LensRunID, f.OrganizationID, f.LensKey,
			f.Category, f.Excerpt, f.LocationHint, f.Details); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Findings) ListByLensRun(ctx context.Context, orgID, lensRunID uuid.UUID) ([]*Finding, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, review_run_id, lens_run_id, organization_id, lens_key, category, excerpt, location_hint, details, created_at
		FROM findings
		WHERE organization_id = $1 AND lens_run_id = $2
		ORDER BY created_at
	`, orgID, lensRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Finding
	for rows.Next() {
		var f Finding
		if err := rows.Scan(
			&f.ID, &f.ReviewRunID, &f.LensRunID, &f.OrganizationID, &f.LensKey,
			&f.Category, &f.Excerpt, &f.LocationHint, &f.Details, &f.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

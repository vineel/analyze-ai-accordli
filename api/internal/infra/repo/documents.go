package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Document struct {
	ID         uuid.UUID
	MatterID   uuid.UUID
	Kind       string // 'original' | 'markdown'
	BlobURL    *string
	ContentMD  *string
	Filename   *string
	SizeBytes  *int64
	SHA256     *string
}

type Documents struct{ pool *pgxpool.Pool }

func (r *Documents) Insert(ctx context.Context, d *Document) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO documents (matter_id, kind, blob_url, content_md, filename, size_bytes, sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, d.MatterID, d.Kind, d.BlobURL, d.ContentMD, d.Filename, d.SizeBytes, d.SHA256).Scan(&d.ID)
}

func (r *Documents) GetByKind(ctx context.Context, matterID uuid.UUID, kind string) (*Document, error) {
	var d Document
	err := r.pool.QueryRow(ctx, `
		SELECT id, matter_id, kind, blob_url, content_md, filename, size_bytes, sha256
		FROM documents
		WHERE matter_id = $1 AND kind = $2
	`, matterID, kind).Scan(
		&d.ID, &d.MatterID, &d.Kind, &d.BlobURL, &d.ContentMD,
		&d.Filename, &d.SizeBytes, &d.SHA256,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &d, err
}

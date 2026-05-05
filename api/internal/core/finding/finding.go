// Package finding persists Findings (one row per Lens output item).
//
// PERMANENT: survives Mocky → Analyze cutover. Stable indexable fields +
// JSONB details so adding Lenses doesn't require ALTER TABLE.
package finding

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

type Finding struct {
	ID             uuid.UUID
	ReviewRunID    uuid.UUID
	LensRunID      uuid.UUID
	OrganizationID uuid.UUID
	LensKey        string
	Category       string
	Excerpt        string
	LocationHint   string
	Details        json.RawMessage
}

type Service interface {
	PersistAll(ctx context.Context, lensRunID uuid.UUID, findings []*Finding) error
	ListByRun(ctx context.Context, orgID, reviewRunID uuid.UUID) ([]*Finding, error)
}

type stub struct{}

func NewStub() Service { return stub{} }

func (stub) PersistAll(context.Context, uuid.UUID, []*Finding) error {
	return errors.New("finding.PersistAll: not implemented")
}

func (stub) ListByRun(context.Context, uuid.UUID, uuid.UUID) ([]*Finding, error) {
	return nil, errors.New("finding.ListByRun: not implemented")
}

// Package billing is the Reserve / Commit / Rollback seam wrapping every
// billable operation.
//
// SoloMocky:    NoopReservation. The seam shape is what matters; Phase
//               Stripe slots the real impl in unchanged.
// Phase Stripe: StripeReservation. Reserve = create reservations row,
//               Commit = ledger -1 + meter event, Rollback = free outcome.
package billing

import (
	"context"

	"github.com/google/uuid"
)

type Reservation struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Units          int
}

type Reserver interface {
	Reserve(ctx context.Context, orgID uuid.UUID, units int) (*Reservation, error)
	Commit(ctx context.Context, r *Reservation) error
	Rollback(ctx context.Context, r *Reservation) error
}

type Noop struct{}

func NewNoop() Noop { return Noop{} }

func (Noop) Reserve(_ context.Context, orgID uuid.UUID, units int) (*Reservation, error) {
	return &Reservation{ID: uuid.New(), OrganizationID: orgID, Units: units}, nil
}

func (Noop) Commit(context.Context, *Reservation) error   { return nil }
func (Noop) Rollback(context.Context, *Reservation) error { return nil }

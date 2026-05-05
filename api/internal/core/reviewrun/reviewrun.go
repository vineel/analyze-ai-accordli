// Package reviewrun is Reviewer's permanent runtime: ReviewRun state
// machine, Prefix builder, Reserve/Commit scope around a Run.
//
// PERMANENT: survives Mocky → Analyze cutover.
package reviewrun

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusPartial   Status = "partial"
	StatusFailed    Status = "failed"
)

type ReviewRun struct {
	ID               uuid.UUID
	MatterID         uuid.UUID
	OrganizationID   uuid.UUID
	Status           Status
	Prefix           string
	PrefixTokenCount int
	ReservationID    uuid.UUID
	Vendor           string
}

type Service interface {
	Start(ctx context.Context, orgID, matterID uuid.UUID) (*ReviewRun, error)
	Get(ctx context.Context, orgID, id uuid.UUID) (*ReviewRun, error)
}

type stub struct{}

func NewStub() Service { return stub{} }

func (stub) Start(context.Context, uuid.UUID, uuid.UUID) (*ReviewRun, error) {
	return nil, errors.New("reviewrun.Start: not implemented")
}

func (stub) Get(context.Context, uuid.UUID, uuid.UUID) (*ReviewRun, error) {
	return nil, errors.New("reviewrun.Get: not implemented")
}

// Package lens runs one Lens template against a ReviewRun's Prefix.
//
// PERMANENT: survives Mocky → Analyze cutover; the Lens *content* is what
// changes (Mocky's two stub Lenses get replaced by Analyze's real set).
package lens

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
	StatusFailed    Status = "failed"
)

type LensRun struct {
	ID              uuid.UUID
	ReviewRunID     uuid.UUID
	OrganizationID  uuid.UUID
	LensKey         string
	LensTemplateSHA string
	Status          Status
	RetryCount      int
	Vendor          string
	FindingCount    int
	ErrorKind       string
}

type Service interface {
	Run(ctx context.Context, lr *LensRun) error
	Get(ctx context.Context, orgID, id uuid.UUID) (*LensRun, error)
}

type stub struct{}

func NewStub() Service { return stub{} }

func (stub) Run(context.Context, *LensRun) error {
	return errors.New("lens.Run: not implemented")
}

func (stub) Get(context.Context, uuid.UUID, uuid.UUID) (*LensRun, error) {
	return nil, errors.New("lens.Get: not implemented")
}

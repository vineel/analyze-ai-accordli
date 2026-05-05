// Package matter holds the Matter lifecycle and lock invariant.
//
// PERMANENT: survives Mocky → Analyze cutover.
package matter

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft  Status = "draft"
	StatusLocked Status = "locked"
)

type Matter struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	DepartmentID   uuid.UUID
	CreatedByUser  uuid.UUID
	Title          string
	Status         Status
}

type Service interface {
	Create(ctx context.Context, orgID, deptID, userID uuid.UUID, title string) (*Matter, error)
	Get(ctx context.Context, orgID, id uuid.UUID) (*Matter, error)
	List(ctx context.Context, orgID, deptID uuid.UUID) ([]*Matter, error)
	Lock(ctx context.Context, orgID, id uuid.UUID) error
}

type stub struct{}

func NewStub() Service { return stub{} }

func (stub) Create(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (*Matter, error) {
	return nil, errors.New("matter.Create: not implemented")
}

func (stub) Get(context.Context, uuid.UUID, uuid.UUID) (*Matter, error) {
	return nil, errors.New("matter.Get: not implemented")
}

func (stub) List(context.Context, uuid.UUID, uuid.UUID) ([]*Matter, error) {
	return nil, errors.New("matter.List: not implemented")
}

func (stub) Lock(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("matter.Lock: not implemented")
}

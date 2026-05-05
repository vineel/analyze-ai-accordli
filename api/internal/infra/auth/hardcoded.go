package auth

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type HardcodedProvider struct {
	Identity Identity
}

func NewHardcoded(userID, orgID, deptID uuid.UUID, email string) *HardcodedProvider {
	return &HardcodedProvider{
		Identity: Identity{
			UserID:         userID,
			OrganizationID: orgID,
			DepartmentID:   deptID,
			Email:          email,
		},
	}
}

func (p *HardcodedProvider) Resolve(_ context.Context, _ *http.Request) (*Identity, error) {
	id := p.Identity
	return &id, nil
}

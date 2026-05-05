// Package auth is the authentication seam.
//
// SoloMocky:        HardcodedProvider returns the Mocky user.
// Phase WorkOS:     WorkOSProvider replaces it. Same interface, different impl.
package auth

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type Identity struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	DepartmentID   uuid.UUID
	Email          string
}

type Provider interface {
	// Resolve produces an Identity from a request. SoloMocky ignores
	// the request entirely and returns the Mocky user.
	Resolve(ctx context.Context, r *http.Request) (*Identity, error)
}

// THROWAWAY: deleted at Mocky → Analyze cutover.
package solomocky

import (
	"context"
	"errors"
)

// Seed creates the Mocky Org + Dept + User if they don't exist.
// Stub today — Phase 2 wires it to a real DB connection.
func Seed(_ context.Context) error {
	return errors.New("solomocky.Seed: not implemented (Phase 2)")
}

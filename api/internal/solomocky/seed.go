// THROWAWAY: deleted at Mocky → Analyze cutover.
package solomocky

import (
	"context"

	"accordli.com/analyze-ai/api/internal/infra/repo"
)

// Seed creates the Mocky Org + Dept + User if they don't exist.
// Idempotent — safe to run on every startup.
func Seed(ctx context.Context, repos *repo.Repos) error {
	return repos.Org.EnsureMockyTrio(
		ctx,
		OrgID, DeptID, UserID,
		OrgName, DeptName, UserEmail,
	)
}

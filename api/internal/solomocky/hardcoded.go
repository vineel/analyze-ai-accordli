// THROWAWAY: deleted at Mocky → Analyze cutover.
//
// Mocky-only constants: the hardcoded Org/Dept/User UUIDs and sample
// credentials. No Scaffolding equivalent — at cutover, the real impl
// behind /infra/auth (WorkOS) supersedes everything here.
package solomocky

import "github.com/google/uuid"

// Stable UUIDs so seed.sh and tests can reference the same rows.
var (
	OrgID        = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	DeptID       = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	UserID       = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	OrgName      = "Mocky Org"
	DeptName     = "Mocky Team"
	UserEmail    = "mocky@accordli.local"
	UserUsername = "mocky"
	UserPassword = "starter" // Mocky-only; replaced by WorkOS at cutover.
)

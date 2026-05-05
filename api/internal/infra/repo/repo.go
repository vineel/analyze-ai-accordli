// Package repo holds the SQL row-mapping for every domain table.
//
// Each Repo is one type per table, taking *pgxpool.Pool. Queries are
// hand-rolled SQL — no ORM, no codegen — because the surface is small
// and the §5 schema is stable. Every read scopes by organization_id;
// the predicate is in place from Day 1 so RLS later is belt-and-
// suspenders, not a retrofit.
package repo

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repos struct {
	Pool        *pgxpool.Pool
	Org         *Orgs
	Matter      *Matters
	Document    *Documents
	ReviewRun   *ReviewRuns
	LensRun     *LensRuns
	Finding     *Findings
}

func New(pool *pgxpool.Pool) *Repos {
	return &Repos{
		Pool:      pool,
		Org:       &Orgs{pool: pool},
		Matter:    &Matters{pool: pool},
		Document:  &Documents{pool: pool},
		ReviewRun: &ReviewRuns{pool: pool},
		LensRun:   &LensRuns{pool: pool},
		Finding:   &Findings{pool: pool},
	}
}

// Package queue is the job-dispatch seam.
//
// SoloMocky:    GoroutineDispatcher (in-process).
// Phase River:  RiverDispatcher replaces it. Same Job and Handler
//               signatures, so the swap is a registration change.
package queue

import "context"

type Job struct {
	ID   string
	Kind string
	Args []byte
}

type Handler func(ctx context.Context, j Job) error

type Dispatcher interface {
	Register(kind string, h Handler)
	Enqueue(ctx context.Context, j Job) error
}

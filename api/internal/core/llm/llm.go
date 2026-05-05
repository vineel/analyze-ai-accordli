// Package llm is the single Go interface every LLM call goes through.
//
// PERMANENT: survives Mocky → Analyze cutover. Today there is one impl
// (Anthropic-direct, stubbed). Foundry and the failover wrapper land in
// later phases behind the same interface.
package llm

import (
	"context"
	"errors"
)

// Block is one Anthropic-shaped content block. The Prefix is sent as a
// block with CacheControl="ephemeral"; the Lens suffix is a second block.
type Block struct {
	Text         string
	CacheControl string // "" or "ephemeral"
}

type Request struct {
	Model       string
	System      string
	Blocks      []Block
	MaxTokens   int
	Temperature float32
}

type Response struct {
	Text         string
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
	Vendor       string // "A" (Foundry) | "B" (Anthropic-direct)
}

type Client interface {
	Complete(ctx context.Context, req Request) (*Response, error)
}

type stub struct{}

func NewStub() Client { return stub{} }

func (stub) Complete(context.Context, Request) (*Response, error) {
	return nil, errors.New("llm.Complete: not implemented")
}

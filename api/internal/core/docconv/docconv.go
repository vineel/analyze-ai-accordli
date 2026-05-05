// Package docconv wraps docx2md-go for the worker's convert step.
//
// PERMANENT: survives Mocky → Analyze cutover. The wrapper exists so Lens
// code never imports the vendored package directly.
package docconv

import (
	"context"
	"errors"
)

type Converter interface {
	DocxToMarkdown(ctx context.Context, docx []byte) (markdown string, err error)
}

type stub struct{}

func NewStub() Converter { return stub{} }

func (stub) DocxToMarkdown(context.Context, []byte) (string, error) {
	return "", errors.New("docconv.DocxToMarkdown: not implemented")
}

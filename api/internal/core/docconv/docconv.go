// Package docconv wraps the in-tree docx2md package for the worker's
// convert step.
//
// PERMANENT: survives Mocky → Analyze cutover. The wrapper exists so
// callers never import docx2md directly — and so the bytes-in /
// string-out shape stays stable even if the underlying library changes.
package docconv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"accordli.com/analyze-ai/api/internal/docx2md"
)

type Converter interface {
	DocxToMarkdown(ctx context.Context, docx []byte) (markdown string, err error)
}

// Pandoc is the docx2md-backed converter. docx2md.Convert is path-in /
// path-out, so we shuttle bytes through a tempdir. Pandoc must be on
// $PATH at runtime.
type Pandoc struct{}

func New() *Pandoc { return &Pandoc{} }

func (Pandoc) DocxToMarkdown(ctx context.Context, in []byte) (string, error) {
	dir, err := os.MkdirTemp("", "docconv-*")
	if err != nil {
		return "", fmt.Errorf("mkdir tmp: %w", err)
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "in.docx")
	dst := filepath.Join(dir, "out.md")
	if err := os.WriteFile(src, in, 0o600); err != nil {
		return "", fmt.Errorf("write tmp docx: %w", err)
	}

	if _, err := docx2md.Convert(ctx, src, dst); err != nil {
		return "", fmt.Errorf("docx2md.Convert: %w", err)
	}

	out, err := os.ReadFile(dst)
	if err != nil {
		return "", fmt.Errorf("read tmp md: %w", err)
	}
	return string(out), nil
}

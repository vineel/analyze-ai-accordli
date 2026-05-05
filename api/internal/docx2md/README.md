# docx2md (vendored)

This package is the in-tree home for `docx2md-go`. The kickoff specified vendoring the source from `~/accordli-dev/docx2md-go/`, but that directory does not exist, and no other working copy turned up under `$HOME` during Phase 0.

## To vendor the real source

1. Locate the real `docx2md-go` working copy.
2. Copy its `*.go` files into this directory.
3. Replace all internal package imports with `accordli.com/analyze-ai/api/internal/docx2md`.
4. Run `cd api && go mod tidy && go vet ./... && go build ./...` to confirm.
5. Update `api/internal/core/docconv/docconv.go` so `DocxToMarkdown` calls `docx2md.Convert` directly.
6. Delete this README.

Phase 0 only requires that the package exists and the rest of the tree builds. No caller invokes `Convert` yet.

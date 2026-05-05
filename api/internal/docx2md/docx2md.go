// Package docx2md is the in-tree home for the docx2md-go converter.
//
// PLACEHOLDER. The real source was not findable on disk during Phase 0
// (kickoff said "likely ~/accordli-dev/docx2md-go/"; no such directory
// exists, and a wider $HOME search turned up nothing). Vendor the real
// package here when its source is located; update imports to
// accordli.com/analyze-ai/api/internal/docx2md and verify the API still
// builds.
//
// Until then, Convert returns a stub error. Phase 0 has no caller — the
// docconv wrapper in /api/internal/core/docconv references this package
// only via a stubbed interface — so the placeholder does not break the
// "buildable skeleton" definition of done.
package docx2md

import "errors"

// Convert turns a .docx blob into Markdown.
//
// PLACEHOLDER: replace with the real implementation when the source is
// vendored.
func Convert(_ []byte) (string, error) {
	return "", errors.New("docx2md.Convert: not yet vendored")
}

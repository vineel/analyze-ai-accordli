// THROWAWAY: deleted at Mocky → Analyze cutover.
package solomocky

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SampleDocxRelPath is the path to the bundled sample agreement,
// relative to the repo root.
const SampleDocxRelPath = "notes/scaffolding/starter-app/mocky-files/sample-agreement-1.docx"

// SampleDocxFilename is the user-facing filename surfaced in the FE
// download buttons.
const SampleDocxFilename = "sample-agreement-1.docx"

// LoadSampleDocx returns the bytes of the bundled sample agreement.
// Walks up from the working directory until the relative path resolves;
// `go run`, `go test`, and the built binary all start in different
// places, so a fixed prefix doesn't fly.
func LoadSampleDocx() ([]byte, error) {
	path, err := resolveRepoFile(SampleDocxRelPath)
	if err != nil {
		return nil, fmt.Errorf("locate sample docx: %w", err)
	}
	return os.ReadFile(path)
}

func resolveRepoFile(rel string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("not found in any ancestor of " + cwd)
}

// THROWAWAY: deleted at Mocky → Analyze cutover.
package solomocky

import "errors"

// SampleDocxPath is the in-repo path to the Mocky sample agreement,
// resolved relative to the repo root at runtime.
const SampleDocxPath = "notes/scaffolding/starter-app/mocky-files/sample-agreement-1.docx"

// LoadSampleDocx returns the bytes of the bundled sample agreement.
// Stub today — Phase 2 wires it to the actual file read + repo-root
// discovery.
func LoadSampleDocx() ([]byte, error) {
	return nil, errors.New("solomocky.LoadSampleDocx: not implemented (Phase 2)")
}

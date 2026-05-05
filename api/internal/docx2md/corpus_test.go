package docx2md

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// requirePandoc skips the test if pandoc is missing or its version doesn't
// match testdata/pandoc-version. Goldens were produced with the pinned
// version; running against another version is not a meaningful comparison.
func requirePandoc(t *testing.T) {
	t.Helper()
	if !pandocOK() {
		want, _ := os.ReadFile("testdata/pandoc-version")
		t.Skipf("pandoc version mismatch: want %q on PATH (see testdata/pandoc-version)",
			strings.TrimSpace(string(want)))
	}
}

var (
	pandocCheckOnce sync.Once
	pandocCheckOK   bool
)

func pandocOK() bool {
	pandocCheckOnce.Do(func() {
		want, err := os.ReadFile("testdata/pandoc-version")
		if err != nil {
			return
		}
		wantLine := strings.TrimSpace(string(want))
		out, err := exec.Command("pandoc", "--version").Output()
		if err != nil {
			return
		}
		gotLine := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
		pandocCheckOK = gotLine == wantLine
	})
	return pandocCheckOK
}

// findCorpusDocx returns the .docx file inside a corpus agreement directory.
// Each agreement-N has exactly one .docx at its top level.
//
// The corpus is not vendored into this repo (it lives in evolver). When the
// directory is absent, the test is skipped so a vanilla `go test ./...`
// from a fresh clone is green. See README.md for the symlink recipe.
func findCorpusDocx(t *testing.T, agreementDir string) string {
	t.Helper()
	entries, err := os.ReadDir(agreementDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("corpus dir %s not present (see README.md)", agreementDir)
		}
		t.Fatalf("read corpus dir %s: %v", agreementDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".docx" {
			return filepath.Join(agreementDir, e.Name())
		}
	}
	t.Fatalf("no .docx in %s", agreementDir)
	return ""
}

// expectedStats is loaded once from testdata/expected_stats.json. The Phase 0
// file may be empty / missing entries; tests that need a stat block skip
// cleanly when the entry is absent.
func loadExpectedStats(t *testing.T) map[string]Stats {
	t.Helper()
	data, err := os.ReadFile("testdata/expected_stats.json")
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Stats{}
		}
		t.Fatalf("read expected_stats.json: %v", err)
	}
	out := map[string]Stats{}
	if len(bytes.TrimSpace(data)) == 0 {
		return out
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parse expected_stats.json: %v", err)
	}
	return out
}

// TestCorpusByteEqual is the Layer A parity test. It runs Convert against
// each agreement's .docx and diffs the output bytes against the committed
// pandoc-numbered golden. Until Phase 5 lands, every case fails because
// Convert is a stub.
func TestCorpusByteEqual(t *testing.T) {
	requirePandoc(t)

	agreements := []string{
		"agreement-1", "agreement-2", "agreement-3", "agreement-4",
		"agreement-5", "agreement-6", "agreement-7", "agreement-8",
		"agreement-9", "agreement-10", "agreement-11", "agreement-12",
		"agreement-13", "agreement-14",
	}

	actualDir := filepath.Join("testdata", "_actual")
	if err := os.MkdirAll(actualDir, 0o755); err != nil {
		t.Fatalf("mkdir _actual: %v", err)
	}

	for _, name := range agreements {
		t.Run(name, func(t *testing.T) {
			agreementDir := filepath.Join("testdata", "corpus", name)
			src := findCorpusDocx(t, agreementDir)
			golden := filepath.Join(agreementDir, "llm", "agreement-from-pandoc-numbered.md")

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden %s: %v", golden, err)
			}

			tmpOut := filepath.Join(actualDir, name+".md")
			_ = os.Remove(tmpOut)

			ctx := context.Background()
			if _, err := Convert(ctx, src, tmpOut); err != nil {
				t.Fatalf("Convert(%s): %v", src, err)
			}

			got, err := os.ReadFile(tmpOut)
			if err != nil {
				t.Fatalf("read actual %s: %v", tmpOut, err)
			}

			if !bytes.Equal(want, got) {
				// Persist actual for offline `diff golden actual` inspection.
				t.Errorf("output differs from %s\nwant %d bytes, got %d bytes\nactual written to %s",
					golden, len(want), len(got), tmpOut)
			}
		})
	}
}

// TestCorpusStats is the Layer B per-agreement Stats assertion. It reads
// testdata/expected_stats.json (captured one-time from the Python pipeline)
// and asserts PreprocessDocx returns the same counts.
func TestCorpusStats(t *testing.T) {
	expected := loadExpectedStats(t)
	if len(expected) == 0 {
		t.Skip("testdata/expected_stats.json is empty; capture from Python before running")
	}

	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			agreementDir := filepath.Join("testdata", "corpus", name)
			src := findCorpusDocx(t, agreementDir)

			tmp, err := os.CreateTemp(t.TempDir(), "docx2md-*.docx")
			if err != nil {
				t.Fatalf("tmp: %v", err)
			}
			tmp.Close()

			got, err := PreprocessDocx(src, tmp.Name())
			if err != nil {
				t.Fatalf("PreprocessDocx: %v", err)
			}
			if got != want {
				t.Errorf("Stats mismatch:\n  want %+v\n  got  %+v", want, got)
			}
		})
	}
}

package lens

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// Templates loads and caches Lens / summary templates from disk. The
// repo is canonical (Reviewer-v2 §"Technical Notes"); we record the
// sha1 of the on-disk bytes as lens_template_sha so a Run is forever
// linked to the prompt that ran it.
//
// "git SHA" in Reviewer-v2 is conceptually the right key. We use a
// content sha1 instead — it's deterministic, doesn't require shelling
// to git, and stays correct for in-flight edits. When we wire a
// proper repo-canonical path later, we can swap the hash source.
type Templates struct {
	Root string // /prompts at the repo root
}

func (t *Templates) Load(subdir, key string) (body, sha string, err error) {
	path := filepath.Join(t.Root, subdir, key+".tmpl")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", path, err)
	}
	sum := sha1.Sum(raw)
	return string(raw), hex.EncodeToString(sum[:]), nil
}

// Render parses tmplBody as a text/template and renders it against
// data. Lens / summary templates are mostly static today; the parser
// gives us {{/* comment */}} blocks and lets us hydrate matter
// metadata into the suffix later without a syntax change.
func Render(tmplBody string, data any) (string, error) {
	t, err := template.New("lens").Parse(tmplBody)
	if err != nil {
		return "", err
	}
	var buf []byte
	bw := writer{&buf}
	if err := t.Execute(&bw, data); err != nil {
		return "", err
	}
	return string(buf), nil
}

type writer struct{ buf *[]byte }

func (w *writer) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

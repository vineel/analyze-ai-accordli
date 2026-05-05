# docx2md (vendored)

Go port of `tools/docx2md` (Python). Applies an XML-driven preprocess to a Word `.docx` so that `pandoc -f docx -t gfm` produces clean, numbered, single-H1 markdown.

## Origin

Vendored from `evolver-accordli`:

```
~/accordli-dev/evolver/docx-files/contracts-enhanced/tools/docx2md-go/
```

The source had `module github.com/accordli/evolver/tools/docx2md-go` with `package docx2md` throughout. Inside the package, no file imports the module path (only an external `github.com/beevik/etree`), so dropping the files here required no edits beyond removing the module wiring (the `go.mod`, the `cmd/docx2md/` CLI, and the host `Makefile` were not vendored). The package now lives at `accordli.com/analyze-ai/api/internal/docx2md`.

## Public API

```go
PreprocessDocx(src, dst string) (Stats, error)              // XML transform only
Convert(ctx, src, dst string) (Stats, error)                 // preprocess + pandoc -f docx -t gfm + heading normalize
NormalizeSectionHeadings(md string) string                   // post-pandoc cleanup
```

`Convert` and the corpus tests shell out to **pandoc**; install it locally before running the corresponding tests.

## docconv adapter

`/api/internal/core/docconv` exposes a bytes-in / string-out `DocxToMarkdown` interface. The real impl will write the input bytes to a temp file, call `docx2md.Convert`, and read the output — that adapter is Phase 2 work.

## Tests

| Test | Needs |
|---|---|
| `TestParseNumberingHandcrafted` and friends in `*_test.go` | nothing (inline XML) |
| `TestParseAgreement1Smoke` (`styles_test.go`) | `testdata/corpus/agreement-1/…docx` |
| `TestCorpusByteEqual`, `TestCorpusStats` (`corpus_test.go`) | matching `pandoc` version + `testdata/corpus/agreement-*` |

The corpus (~100MB of confidential agreements) lives in the evolver repo and is **not** vendored. The corpus tests skip cleanly when the corpus isn't present and pandoc isn't on PATH.

To run the corpus tests locally, symlink the corpus into `testdata/corpus/`:

```sh
cd api/internal/docx2md/testdata
ln -s ~/accordli-dev/evolver/docx-files/contracts-enhanced/agreement-1 corpus/agreement-1
# … repeat for agreement-2 … agreement-14, or symlink the whole parent
```

`testdata/pandoc-version` and `testdata/expected_stats.json` are checked in (small fixtures the tests reference).

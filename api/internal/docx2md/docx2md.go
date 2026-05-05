// Package docx2md is a Go port of tools/docx2md (Python). It applies an
// XML-driven preprocess to a Word .docx so that pandoc -f docx -t gfm
// produces clean, numbered, single-H1 markdown.
package docx2md

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
)

// Stats is the per-run counter set returned by PreprocessDocx and Convert.
//
//	TitleToH1:      paragraphs whose Title style was rewritten to Heading1
//	Demoted:        heading-styled paragraphs whose style was dropped
//	Promoted:       plain paragraphs whose text was a section title and got Heading1
//	LabelsInjected: list paragraphs that got a literal numbering label inlined
//	LabelsSkipped:  list paragraphs whose numId/level was undefined in numbering.xml
type Stats struct {
	TitleToH1      int `json:"title_to_h1"`
	Demoted        int `json:"demoted"`
	Promoted       int `json:"promoted"`
	LabelsInjected int `json:"labels_injected"`
	LabelsSkipped  int `json:"labels_skipped"`
}

// PreprocessDocx reads src as a .docx, applies the XML-driven transform,
// and writes a transformed .docx to dst preserving zip member order.
func PreprocessDocx(src, dst string) (Stats, error) {
	zin, err := zip.OpenReader(src)
	if err != nil {
		return Stats{}, fmt.Errorf("open src docx: %w", err)
	}
	defer zin.Close()

	type member struct {
		header *zip.FileHeader
		data   []byte
	}
	members := make([]member, 0, len(zin.File))
	var numberingXML, stylesXML, documentXML []byte
	for _, f := range zin.File {
		rc, err := f.Open()
		if err != nil {
			return Stats{}, fmt.Errorf("open zip member %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return Stats{}, fmt.Errorf("read zip member %s: %w", f.Name, err)
		}
		// Copy the FileHeader so we can mutate timestamps without touching
		// the source reader.
		hdr := f.FileHeader
		members = append(members, member{header: &hdr, data: data})
		switch f.Name {
		case "word/numbering.xml":
			numberingXML = data
		case "word/styles.xml":
			stylesXML = data
		case "word/document.xml":
			documentXML = data
		}
	}

	// If the docx has no numbering.xml AND no styles.xml, the transform
	// has nothing to do — copy bytes verbatim. (Matches Python.)
	if numberingXML == nil && stylesXML == nil {
		return Stats{}, copyFile(src, dst)
	}

	var (
		abstracts abstractNums
		n2a       numToAbstract
		styleMap  map[string]styleInfo
	)
	if numberingXML != nil {
		abstracts, n2a, err = parseNumbering(numberingXML)
		if err != nil {
			return Stats{}, fmt.Errorf("parse numbering.xml: %w", err)
		}
	} else {
		abstracts, n2a = abstractNums{}, numToAbstract{}
	}
	if stylesXML != nil {
		styleMap, err = parseStyles(stylesXML)
		if err != nil {
			return Stats{}, fmt.Errorf("parse styles.xml: %w", err)
		}
	} else {
		styleMap = map[string]styleInfo{}
	}

	if documentXML == nil {
		return Stats{}, fmt.Errorf("docx is missing word/document.xml")
	}
	newDocXML, stats, err := processDocument(documentXML, abstracts, n2a, styleMap)
	if err != nil {
		return Stats{}, fmt.Errorf("process document.xml: %w", err)
	}

	// Write zip preserving original member order.
	out, err := os.Create(dst)
	if err != nil {
		return Stats{}, fmt.Errorf("create dst: %w", err)
	}
	defer out.Close()
	zout := zip.NewWriter(out)
	for _, m := range members {
		data := m.data
		if m.header.Name == "word/document.xml" {
			data = newDocXML
		}
		// Preserve the compression method that the source used. We force
		// a clean header so size/CRC are recomputed.
		hdr := &zip.FileHeader{
			Name:     m.header.Name,
			Method:   m.header.Method,
			Modified: m.header.Modified,
		}
		w, err := zout.CreateHeader(hdr)
		if err != nil {
			zout.Close()
			return Stats{}, fmt.Errorf("create zip member %s: %w", m.header.Name, err)
		}
		if _, err := w.Write(data); err != nil {
			zout.Close()
			return Stats{}, fmt.Errorf("write zip member %s: %w", m.header.Name, err)
		}
	}
	if err := zout.Close(); err != nil {
		return Stats{}, fmt.Errorf("close zip writer: %w", err)
	}
	return stats, nil
}

// Convert runs the full pipeline: PreprocessDocx → pandoc -f docx -t gfm
// → NormalizeSectionHeadings → write markdown to dst.
func Convert(ctx context.Context, src, dst string) (Stats, error) {
	tmp, err := os.CreateTemp("", "docx2md-*.docx")
	if err != nil {
		return Stats{}, fmt.Errorf("create tmp docx: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	stats, err := PreprocessDocx(src, tmpPath)
	if err != nil {
		return stats, err
	}
	md, err := runPandoc(ctx, tmpPath)
	if err != nil {
		return stats, fmt.Errorf("pandoc: %w", err)
	}
	md = []byte(NormalizeSectionHeadings(string(md)))
	if err := os.WriteFile(dst, md, 0o644); err != nil {
		return stats, fmt.Errorf("write markdown: %w", err)
	}
	return stats, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

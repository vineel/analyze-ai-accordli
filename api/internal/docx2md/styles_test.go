package docx2md

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParseNumberingHandcrafted(t *testing.T) {
	xml := []byte(`<?xml version="1.0"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:abstractNum w:abstractNumId="0">
    <w:lvl w:ilvl="0">
      <w:start w:val="1"/>
      <w:numFmt w:val="decimal"/>
      <w:lvlText w:val="%1."/>
    </w:lvl>
    <w:lvl w:ilvl="1">
      <w:start w:val="1"/>
      <w:numFmt w:val="decimal"/>
      <w:lvlText w:val="%1.%2"/>
    </w:lvl>
    <w:lvl w:ilvl="2">
      <w:numFmt w:val="lowerLetter"/>
      <w:lvlText w:val="(%3)"/>
    </w:lvl>
  </w:abstractNum>
  <w:abstractNum w:abstractNumId="2">
    <w:lvl w:ilvl="0">
      <w:lvlText w:val="-"/>
    </w:lvl>
  </w:abstractNum>
  <w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
  <w:num w:numId="3"><w:abstractNumId w:val="2"/></w:num>
</w:numbering>`)
	abs, n2a, err := parseNumbering(xml)
	if err != nil {
		t.Fatalf("parseNumbering err: %v", err)
	}

	if got := n2a["1"]; got != "0" {
		t.Errorf("numToAbstract[1] = %q, want %q", got, "0")
	}
	if got := n2a["3"]; got != "2" {
		t.Errorf("numToAbstract[3] = %q, want %q", got, "2")
	}

	abs0, ok := abs["0"]
	if !ok {
		t.Fatalf("abstract 0 missing")
	}
	if abs0["0"] != (levelMeta{Start: 1, NumFmt: "decimal", LvlText: "%1."}) {
		t.Errorf("abs[0][0] = %+v", abs0["0"])
	}
	if abs0["1"] != (levelMeta{Start: 1, NumFmt: "decimal", LvlText: "%1.%2"}) {
		t.Errorf("abs[0][1] = %+v", abs0["1"])
	}
	// Defaults: missing start → 1, missing numFmt → decimal, missing lvlText → %1.
	if abs0["2"] != (levelMeta{Start: 1, NumFmt: "lowerLetter", LvlText: "(%3)"}) {
		t.Errorf("abs[0][2] = %+v", abs0["2"])
	}
	abs2 := abs["2"]
	if abs2["0"] != (levelMeta{Start: 1, NumFmt: "decimal", LvlText: "-"}) {
		t.Errorf("abs[2][0] all-defaults = %+v", abs2["0"])
	}
}

func TestParseStylesHandcrafted(t *testing.T) {
	xml := []byte(`<?xml version="1.0"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:styleId="Heading1">
    <w:name w:val="heading 1"/>
  </w:style>
  <w:style w:styleId="MyHeading">
    <w:name w:val="Custom Heading"/>
    <w:basedOn w:val="Heading1"/>
  </w:style>
  <w:style w:styleId="Heading2">
    <w:name w:val="heading 2"/>
  </w:style>
  <w:style w:styleId="TitleStyle">
    <w:name w:val="Title"/>
  </w:style>
  <w:style w:styleId="ListPara">
    <w:name w:val="ListPara"/>
    <w:pPr>
      <w:numPr>
        <w:numId w:val="3"/>
        <w:ilvl w:val="2"/>
      </w:numPr>
    </w:pPr>
  </w:style>
  <w:style w:styleId="DerivedList">
    <w:name w:val="Derived"/>
    <w:basedOn w:val="ListPara"/>
  </w:style>
  <w:style w:styleId="LoopA">
    <w:name w:val="LoopA"/>
    <w:basedOn w:val="LoopB"/>
  </w:style>
  <w:style w:styleId="LoopB">
    <w:name w:val="LoopB"/>
    <w:basedOn w:val="LoopA"/>
  </w:style>
</w:styles>`)
	m, err := parseStyles(xml)
	if err != nil {
		t.Fatalf("parseStyles err: %v", err)
	}

	cases := []struct {
		sid   string
		level int
		title bool
		num   string
		ilvl  string
		hasN  bool
	}{
		{"Heading1", 1, false, "", "", false},
		{"MyHeading", 1, false, "", "", false}, // inherited via basedOn
		{"Heading2", 2, false, "", "", false},
		{"TitleStyle", 0, true, "", "", false},
		{"ListPara", 0, false, "3", "2", true},
		{"DerivedList", 0, false, "3", "2", true}, // inherited numPr
		// Cycles must not infinite-loop; LoopA/LoopB have no heading or numPr.
		{"LoopA", 0, false, "", "", false},
		{"LoopB", 0, false, "", "", false},
	}
	for _, c := range cases {
		got, ok := m[c.sid]
		if !ok {
			t.Errorf("style %s missing", c.sid)
			continue
		}
		if got.HeadingLevel != c.level {
			t.Errorf("%s.HeadingLevel = %d, want %d", c.sid, got.HeadingLevel, c.level)
		}
		if got.IsTitle != c.title {
			t.Errorf("%s.IsTitle = %v, want %v", c.sid, got.IsTitle, c.title)
		}
		if got.NumID != c.num {
			t.Errorf("%s.NumID = %q, want %q", c.sid, got.NumID, c.num)
		}
		if got.Ilvl != c.ilvl {
			t.Errorf("%s.Ilvl = %q, want %q", c.sid, got.Ilvl, c.ilvl)
		}
		if got.HasNumID != c.hasN {
			t.Errorf("%s.HasNumID = %v, want %v", c.sid, got.HasNumID, c.hasN)
		}
	}

	// findHeading1StyleID picks the styleId whose name is exactly "heading 1".
	if h1 := findHeading1StyleID(m); h1 != "Heading1" {
		t.Errorf("findHeading1StyleID = %q, want %q", h1, "Heading1")
	}
	if got := findHeading1StyleID(map[string]styleInfo{}); got != "Heading1" {
		t.Errorf("findHeading1StyleID(empty) = %q, want fallback Heading1", got)
	}
}

// TestParseAgreement1Smoke reads numbering.xml and styles.xml from the
// agreement-1 docx zip and asserts that the parsers don't error and produce
// non-empty maps.
func TestParseAgreement1Smoke(t *testing.T) {
	docxPath := filepath.Join(
		"testdata", "corpus", "agreement-1",
		"01_MobileDistribution_PixelRush-Broadlaunch.docx",
	)
	if _, err := os.Stat(docxPath); os.IsNotExist(err) {
		// The corpus lives in the evolver repo (~100MB of confidential
		// agreements) and is not vendored. See README.md for how to
		// symlink it into testdata/corpus to run this test locally.
		t.Skip("testdata/corpus not present (see api/internal/docx2md/README.md)")
	}
	r, err := zip.OpenReader(docxPath)
	if err != nil {
		t.Fatalf("open docx: %v", err)
	}
	defer r.Close()

	read := func(name string) []byte {
		for _, f := range r.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					t.Fatalf("open %s: %v", name, err)
				}
				defer rc.Close()
				b, err := io.ReadAll(rc)
				if err != nil {
					t.Fatalf("read %s: %v", name, err)
				}
				return b
			}
		}
		t.Fatalf("%s not found in docx", name)
		return nil
	}

	abs, n2a, err := parseNumbering(read("word/numbering.xml"))
	if err != nil {
		t.Fatalf("parseNumbering: %v", err)
	}
	if len(abs) == 0 {
		t.Errorf("expected at least one abstractNum, got 0")
	}
	if len(n2a) == 0 {
		t.Errorf("expected at least one numId mapping, got 0")
	}

	styles, err := parseStyles(read("word/styles.xml"))
	if err != nil {
		t.Fatalf("parseStyles: %v", err)
	}
	if len(styles) == 0 {
		t.Errorf("expected at least one style, got 0")
	}
	// agreement-1 has H1/H2 styles.
	hasH1 := false
	for _, info := range styles {
		if info.HeadingLevel == 1 {
			hasH1 = true
		}
	}
	if !hasH1 {
		t.Errorf("expected at least one Heading 1 style in agreement-1")
	}
}

package docx2md

import (
	"strings"

	"github.com/beevik/etree"
)

// cellText returns the visible text of a <w:tc> table cell.
//
// Adjacent <w:t> runs within a paragraph join with no separator (Word
// renders them as a continuous string); separate paragraphs join with a
// single space. The result is whitespace-trimmed.
func cellText(tc *etree.Element) string {
	var paragraphs []string
	for _, p := range tc.FindElements(".//w:p") {
		var parts []string
		for _, t := range p.FindElements(".//w:t") {
			parts = append(parts, t.Text())
		}
		joined := strings.TrimSpace(strings.Join(parts, ""))
		if joined != "" {
			paragraphs = append(paragraphs, joined)
		}
	}
	return strings.TrimSpace(strings.Join(paragraphs, " "))
}

// makeKVParagraph builds a <w:p> rendering as `**label** value`.
//
// Either side may be empty. xml:space="preserve" on the <w:t> elements
// preserves the leading space we inject between label and value.
func makeKVParagraph(label, value string) *etree.Element {
	p := etree.NewElement("w:p")
	if label != "" {
		r := p.CreateElement("w:r")
		rpr := r.CreateElement("w:rPr")
		rpr.CreateElement("w:b")
		t := r.CreateElement("w:t")
		t.CreateAttr("xml:space", "preserve")
		t.SetText(label)
	}
	if value != "" {
		r := p.CreateElement("w:r")
		t := r.CreateElement("w:t")
		t.CreateAttr("xml:space", "preserve")
		if label != "" {
			t.SetText(" " + value)
		} else {
			t.SetText(value)
		}
	}
	return p
}

// unrollComplexTables replaces "form-style" tables (those containing at
// least one entirely-empty column acting as a visual spacer) with a
// sequence of `**Label:** Value` paragraphs. Returns the count of tables
// unrolled.
//
// Tables without empty spacer columns are left alone — those are real
// tabular data and worth preserving as markdown tables.
func unrollComplexTables(root *etree.Element) int {
	unrolled := 0
	tables := root.FindElements(".//w:tbl")
	for _, tbl := range tables {
		rows := tbl.SelectElements("w:tr")
		if len(rows) == 0 {
			continue
		}
		var rowsData [][]string
		for _, r := range rows {
			cells := r.SelectElements("w:tc")
			row := make([]string, len(cells))
			for i, c := range cells {
				row[i] = cellText(c)
			}
			rowsData = append(rowsData, row)
		}
		nCols := 0
		for _, r := range rowsData {
			if len(r) > nCols {
				nCols = len(r)
			}
		}
		if nCols < 3 {
			continue // 1- or 2-column tables stay as tables
		}
		emptyCols := map[int]bool{}
		for c := 0; c < nCols; c++ {
			allEmpty := true
			for _, r := range rowsData {
				v := ""
				if c < len(r) {
					v = r[c]
				}
				if v != "" {
					allEmpty = false
					break
				}
			}
			if allEmpty {
				emptyCols[c] = true
			}
		}
		if len(emptyCols) == 0 {
			continue
		}
		// Group consecutive non-empty columns.
		var groups [][]int
		var current []int
		for c := 0; c < nCols; c++ {
			if emptyCols[c] {
				if len(current) > 0 {
					groups = append(groups, current)
					current = nil
				}
			} else {
				current = append(current, c)
			}
		}
		if len(current) > 0 {
			groups = append(groups, current)
		}
		// Generate replacement paragraphs.
		var newParagraphs []*etree.Element
		for gi, grp := range groups {
			if gi > 0 {
				// Blank paragraph between column groups for visual break.
				newParagraphs = append(newParagraphs, etree.NewElement("w:p"))
			}
			for _, row := range rowsData {
				cells := make([]string, len(grp))
				for i, c := range grp {
					if c < len(row) {
						cells[i] = row[c]
					}
				}
				if !anyNonEmpty(cells) {
					continue
				}
				if len(cells) >= 2 {
					label := cells[0]
					var rest []string
					for _, c := range cells[1:] {
						if c != "" {
							rest = append(rest, c)
						}
					}
					value := strings.Join(rest, " ")
					if label == "" && value != "" {
						// Continuation: append to previous paragraph's value.
						if n := len(newParagraphs); n > 0 && len(newParagraphs[n-1].ChildElements()) > 0 {
							r := newParagraphs[n-1].CreateElement("w:r")
							t := r.CreateElement("w:t")
							t.CreateAttr("xml:space", "preserve")
							t.SetText(" " + value)
						} else {
							newParagraphs = append(newParagraphs, makeKVParagraph("", value))
						}
					} else if label != "" || value != "" {
						newParagraphs = append(newParagraphs, makeKVParagraph(label, value))
					}
				} else {
					if cells[0] != "" {
						newParagraphs = append(newParagraphs, makeKVParagraph("", cells[0]))
					}
				}
			}
		}
		// Replace the table with the new paragraphs.
		parent := tbl.Parent()
		if parent == nil {
			continue
		}
		idx := tbl.Index()
		parent.RemoveChild(tbl)
		for i, np := range newParagraphs {
			parent.InsertChildAt(idx+i, np)
		}
		unrolled++
	}
	return unrolled
}

func anyNonEmpty(s []string) bool {
	for _, v := range s {
		if v != "" {
			return true
		}
	}
	return false
}

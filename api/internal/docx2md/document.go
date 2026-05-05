package docx2md

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// Top-level section title heuristic: starts with `N.` (single dot, no
// nested decimal), then space, then a non-space char.
var (
	sectionHeaderRE    = regexp.MustCompile(`^\s*\d+\.\s+\S`)
	subsectionHeaderRE = regexp.MustCompile(`^\s*\d+\.\d`)
)

// getNumPr resolves the paragraph's effective list numbering:
//   - inline <w:pPr><w:numPr> takes precedence,
//   - otherwise (if styleMap is non-nil) the paragraph's pStyle's
//     style-chain numPr is used,
//   - otherwise (numID="", ilvl="", fromStyle=false).
//
// fromStyle=true means the caller must use overrideNumPrOff to suppress
// pandoc's auto-list (the paragraph itself has no numPr to remove).
func getNumPr(p *etree.Element, styleMap map[string]styleInfo) (numID, ilvl string, fromStyle bool) {
	pPr := p.SelectElement("w:pPr")
	if pPr != nil {
		// Inline <w:numPr> wins, even if it carries no numId — that case
		// is the OOXML way of suppressing style-inherited numbering, and
		// the caller will skip it on the empty-numID test.
		if numPr := pPr.SelectElement("w:numPr"); numPr != nil {
			ilvlVal := "0"
			numIDVal := ""
			if nid := numPr.SelectElement("w:numId"); nid != nil {
				numIDVal = nid.SelectAttrValue("w:val", "")
			}
			if il := numPr.SelectElement("w:ilvl"); il != nil {
				ilvlVal = il.SelectAttrValue("w:val", "")
			}
			return numIDVal, ilvlVal, false
		}
	}
	if styleMap != nil && pPr != nil {
		if styleEl := pPr.SelectElement("w:pStyle"); styleEl != nil {
			sid := styleEl.SelectAttrValue("w:val", "")
			if info, ok := styleMap[sid]; ok && info.HasNumID {
				il := info.Ilvl
				if il == "" {
					il = "0"
				}
				return info.NumID, il, true
			}
		}
	}
	return "", "", false
}

// overrideNumPrOff replaces the paragraph's numPr with an explicit
// <w:numPr><w:numId w:val="0"/></w:numPr>. numId=0 is the OOXML signal
// for "no list" — it overrides any style-inherited numbering.
func overrideNumPrOff(p *etree.Element) {
	pPr := p.SelectElement("w:pPr")
	if pPr == nil {
		pPr = etree.NewElement("w:pPr")
		p.InsertChildAt(0, pPr)
	}
	for _, el := range pPr.ChildElements() {
		if el.Space == "w" && el.Tag == "numPr" {
			pPr.RemoveChild(el)
		}
	}
	numPr := pPr.CreateElement("w:numPr")
	nid := numPr.CreateElement("w:numId")
	nid.CreateAttr("w:val", "0")
}

// removeNumPr drops any <w:numPr> child from the paragraph's pPr. The pPr
// itself is left in place.
func removeNumPr(p *etree.Element) {
	pPr := p.SelectElement("w:pPr")
	if pPr == nil {
		return
	}
	for _, el := range pPr.ChildElements() {
		if el.Space == "w" && el.Tag == "numPr" {
			pPr.RemoveChild(el)
		}
	}
}

// injectLabelText prepends a literal <w:r><w:t xml:space="preserve">label </w:t></w:r>
// to the paragraph. The trailing space ensures pandoc keeps the label
// separate from following inline text.
func injectLabelText(p *etree.Element, label string) {
	if label == "" {
		return
	}
	newR := etree.NewElement("w:r")
	newT := newR.CreateElement("w:t")
	newT.CreateAttr("xml:space", "preserve")
	newT.SetText(label + " ")

	insertAt := 0
	for i, child := range p.ChildElements() {
		if child.Space == "w" && child.Tag == "pPr" {
			insertAt = i + 1
			break
		}
	}
	p.InsertChildAt(insertAt, newR)
}

// normalizeStyles applies the heading-level normalization rules (see core.py
// docstring) and returns (titleToH1, demoted, promoted) counts.
func normalizeStyles(root *etree.Element, styleMap map[string]styleInfo) (titleToH1, demoted, promoted int) {
	h1ID := findHeading1StyleID(styleMap)

	for _, p := range root.FindElements(".//w:p") {
		stripIndent(p)
		text := strings.TrimSpace(getPText(p))
		style := getPStyle(p)
		var info styleInfo
		var ok bool
		if style != "" {
			info, ok = styleMap[style]
		}

		// Title → Heading 1
		if ok && info.IsTitle {
			setPStyle(p, h1ID)
			stripRunBold(p)
			titleToH1++
			continue
		}

		// Heading 2..9 → demote
		if ok && info.HeadingLevel >= 2 {
			removePStyle(p)
			demoted++
			continue
		}

		// Heading 1 whose text isn't a section header (or *is* a subsection) → demote
		if ok && info.HeadingLevel == 1 {
			if !sectionHeaderRE.MatchString(text) || subsectionHeaderRE.MatchString(text) {
				removePStyle(p)
				demoted++
			}
			continue
		}

		// Plain paragraph that looks like a section title → promote
		if sectionHeaderRE.MatchString(text) &&
			!subsectionHeaderRE.MatchString(text) &&
			len([]rune(text)) < 100 {
			setPStyle(p, h1ID)
			stripRunBold(p)
			promoted++
		}
	}
	return
}

// processDocument is the orchestrator. Two passes:
//
//	Pass A — style-inherited numbering. Runs BEFORE normalizeStyles so the
//	         paragraph still has its pStyle (so the section-title heuristic
//	         can pick up the injected `1. ` prefix).
//	normalizeStyles
//	Pass B — inline numPr. Runs AFTER normalizeStyles so promote/demote
//	         doesn't overlap with already-numbered list paragraphs.
//
// Both passes share one counters map keyed by numId.
func processDocument(documentXML []byte, abstracts abstractNums, n2a numToAbstract, styleMap map[string]styleInfo) ([]byte, Stats, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(documentXML); err != nil {
		return nil, Stats{}, err
	}
	root := doc.Root()
	if root == nil {
		return documentXML, Stats{}, nil
	}

	stripBookmarks(root)
	unrollComplexTables(root)

	counters := map[string]map[string]int{}
	var injected, skipped int

	injectOne := func(p *etree.Element, numID, ilvl string, fromStyle bool) {
		abstractID, ok := n2a[numID]
		if !ok {
			skipped++
			return
		}
		levelsMeta, ok := abstracts[abstractID]
		if !ok {
			skipped++
			return
		}
		meta, ok := levelsMeta[ilvl]
		if !ok {
			skipped++
			return
		}
		// Bullets: leave numPr alone so pandoc emits `- text`.
		if meta.NumFmt == "bullet" {
			return
		}
		cset, exists := counters[numID]
		if !exists {
			cset = map[string]int{}
			counters[numID] = cset
		}
		ilvlInt, err := strconv.Atoi(ilvl)
		if err != nil {
			skipped++
			return
		}
		// Back-fill shallower levels that haven't started yet.
		for shallower := 0; shallower < ilvlInt; shallower++ {
			key := strconv.Itoa(shallower)
			if cset[key] == 0 {
				start := 1
				if m, ok2 := levelsMeta[key]; ok2 {
					start = m.Start
				}
				cset[key] = start
			}
		}
		// First time we see this level: prime it to (start - 1), so the
		// `+= 1` below brings it to start.
		if _, seen := cset[ilvl]; !seen {
			cset[ilvl] = meta.Start - 1
		}
		cset[ilvl]++
		// Reset deeper levels.
		for k := range cset {
			if kInt, err := strconv.Atoi(k); err == nil && kInt > ilvlInt {
				cset[k] = 0
			}
		}
		label := renderLabel(meta.LvlText, levelsMeta, cset)
		if label != "" {
			injectLabelText(p, label)
			injected++
		}
		if fromStyle {
			overrideNumPrOff(p)
		} else {
			removeNumPr(p)
		}
	}

	// Pass A — style-inherited numbering only.
	for _, p := range root.FindElements(".//w:p") {
		numID, ilvl, fromStyle := getNumPr(p, styleMap)
		if numID == "" || numID == "0" || !fromStyle {
			continue
		}
		injectOne(p, numID, ilvl, true)
	}

	// Style normalization.
	titleToH1, demoted, promoted := normalizeStyles(root, styleMap)

	// Pass B — inline numPr.
	for _, p := range root.FindElements(".//w:p") {
		numID, ilvl, _ := getNumPr(p, nil)
		if numID == "" || numID == "0" {
			continue
		}
		injectOne(p, numID, ilvl, false)
	}

	// Serialize. We emit the canonical OOXML XML declaration ourselves to
	// match the Python pipeline's invariant output.
	doc.WriteSettings.CanonicalEndTags = false
	body, err := doc.WriteToBytes()
	if err != nil {
		return nil, Stats{}, err
	}
	body = stripXMLDecl(body)
	out := append([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"), body...)

	return out, Stats{
		TitleToH1:      titleToH1,
		Demoted:        demoted,
		Promoted:       promoted,
		LabelsInjected: injected,
		LabelsSkipped:  skipped,
	}, nil
}

// stripXMLDecl removes a leading <?xml ... ?> processing instruction (and
// any trailing whitespace) from an etree-serialized blob, so we can prepend
// our own canonical declaration.
func stripXMLDecl(b []byte) []byte {
	s := string(b)
	if !strings.HasPrefix(s, "<?xml") {
		return b
	}
	end := strings.Index(s, "?>")
	if end < 0 {
		return b
	}
	rest := s[end+2:]
	rest = strings.TrimLeft(rest, "\r\n\t ")
	return []byte(rest)
}

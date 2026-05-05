package docx2md

import "github.com/beevik/etree"

// Paragraph helpers. These mirror the underscore-prefixed helpers in the
// Python core: each takes a <w:p> Element and either reads or mutates its
// pPr / runs / inline children.
//
// They live in bookmarks.go for parity with the Python file split, but the
// stripBookmarks function and these helpers are independently useful.

// getPText concatenates run text in document order. <w:tab/> renders as a
// tab and <w:br/> as a newline, matching Python _get_p_text.
func getPText(p *etree.Element) string {
	var b []byte
	var walk func(el *etree.Element)
	walk = func(el *etree.Element) {
		for _, child := range el.ChildElements() {
			full := child.Space + ":" + child.Tag
			switch full {
			case "w:t":
				b = append(b, child.Text()...)
			case "w:tab":
				b = append(b, '\t')
			case "w:br":
				b = append(b, '\n')
			}
			walk(child)
		}
	}
	walk(p)
	return string(b)
}

// getPStyle returns the val of <w:pPr><w:pStyle>, or "" if absent.
func getPStyle(p *etree.Element) string {
	pPr := p.SelectElement("w:pPr")
	if pPr == nil {
		return ""
	}
	s := pPr.SelectElement("w:pStyle")
	if s == nil {
		return ""
	}
	return s.SelectAttrValue("w:val", "")
}

// setPStyle sets <w:pPr><w:pStyle w:val="val"/>, creating intermediate
// elements as needed.
func setPStyle(p *etree.Element, val string) {
	pPr := p.SelectElement("w:pPr")
	if pPr == nil {
		pPr = etree.NewElement("w:pPr")
		p.InsertChildAt(0, pPr)
	}
	s := pPr.SelectElement("w:pStyle")
	if s == nil {
		s = etree.NewElement("w:pStyle")
		pPr.InsertChildAt(0, s)
	}
	s.RemoveAttr("w:val")
	s.CreateAttr("w:val", val)
}

// removePStyle drops the inline pStyle if any. The pPr element stays.
func removePStyle(p *etree.Element) {
	pPr := p.SelectElement("w:pPr")
	if pPr == nil {
		return
	}
	if s := pPr.SelectElement("w:pStyle"); s != nil {
		pPr.RemoveChild(s)
	}
}

// stripRunBold removes <w:b/> from every run-properties (<w:rPr>) descendant
// of p, matching Python _strip_run_bold. Only "on" forms are removed (val
// absent or true/1/on).
func stripRunBold(p *etree.Element) {
	for _, rPr := range p.FindElements(".//w:rPr") {
		for _, el := range rPr.ChildElements() {
			if el.Space == "w" && el.Tag == "b" {
				v := el.SelectAttrValue("w:val", "")
				if v == "" || v == "true" || v == "1" || v == "on" ||
					v == "True" || v == "On" || v == "TRUE" || v == "ON" {
					rPr.RemoveChild(el)
				}
			}
		}
	}
}

// stripIndent removes any <w:ind> children from the paragraph's pPr.
func stripIndent(p *etree.Element) {
	pPr := p.SelectElement("w:pPr")
	if pPr == nil {
		return
	}
	for _, el := range pPr.ChildElements() {
		if el.Space == "w" && el.Tag == "ind" {
			pPr.RemoveChild(el)
		}
	}
}

// stripBookmarks removes <w:bookmarkStart> and <w:bookmarkEnd> elements
// anywhere in the tree. Word bookmarks render as <span class="anchor"/> in
// pandoc gfm; cross-reference link text is unaffected because it comes from
// the field's cached value.
func stripBookmarks(root *etree.Element) {
	for _, tag := range []string{"w:bookmarkStart", "w:bookmarkEnd"} {
		for _, el := range root.FindElements(".//" + tag) {
			if parent := el.Parent(); parent != nil {
				parent.RemoveChild(el)
			}
		}
	}
}

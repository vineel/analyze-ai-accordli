package docx2md

import (
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// fmtNumber renders n in the OOXML numFmt named by format. Unknown formats
// fall back to decimal; bullet/none render as empty.
func fmtNumber(n int, format string) string {
	switch format {
	case "decimal":
		return strconv.Itoa(n)
	case "lowerLetter":
		return alpha(n, true)
	case "upperLetter":
		return alpha(n, false)
	case "lowerRoman":
		return strings.ToLower(roman(n))
	case "upperRoman":
		return roman(n)
	case "none", "bullet":
		return ""
	default:
		return strconv.Itoa(n)
	}
}

// alpha renders n as a base-26 letter sequence: 1→a, 26→z, 27→aa …
// (matches the Python helper; differs from Word's exact behavior past 26 but
// that range is rare in legal contracts).
func alpha(n int, lower bool) string {
	if n <= 0 {
		return ""
	}
	base := byte('A')
	if lower {
		base = 'a'
	}
	var b []byte
	for n > 0 {
		n--
		b = append([]byte{base + byte(n%26)}, b...)
		n /= 26
	}
	return string(b)
}

// roman renders n in upper-case Roman numerals. Returns "" for n <= 0.
func roman(n int) string {
	if n <= 0 {
		return ""
	}
	type pair struct {
		v int
		s string
	}
	table := []pair{
		{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"},
		{100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"},
		{10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"},
	}
	var b strings.Builder
	for _, p := range table {
		for n >= p.v {
			b.WriteString(p.s)
			n -= p.v
		}
	}
	return b.String()
}

// levelMeta is the per-level metadata extracted from numbering.xml.
type levelMeta struct {
	Start   int
	NumFmt  string
	LvlText string
}

// renderLabel substitutes %1..%9 in lvlText with the formatted counter for
// the corresponding level. Counters are keyed by stringified level index
// ("0".."8") to match the Python pipeline.
func renderLabel(lvlText string, levelsMeta map[string]levelMeta, counters map[string]int) string {
	out := lvlText
	for i := 0; i < 9; i++ {
		token := "%" + strconv.Itoa(i+1)
		if !strings.Contains(out, token) {
			continue
		}
		key := strconv.Itoa(i)
		n := counters[key]
		if meta, ok := levelsMeta[key]; ok {
			out = strings.ReplaceAll(out, token, fmtNumber(n, meta.NumFmt))
		} else {
			out = strings.ReplaceAll(out, token, strconv.Itoa(n))
		}
	}
	return out
}

// abstractNums maps abstractNumId → ilvl → levelMeta.
type abstractNums map[string]map[string]levelMeta

// numToAbstract maps numId → abstractNumId.
type numToAbstract map[string]string

// parseNumbering parses the contents of word/numbering.xml.
//
// Returns:
//   - abstracts: abstractNumId → ilvl → {start, numFmt, lvlText}
//   - resolve:   numId → abstractNumId
//
// All ids are stringified to match the Python pipeline's keys. Missing
// elements receive defaults: start=1, numFmt="decimal", lvlText="%1.".
func parseNumbering(numberingXML []byte) (abstractNums, numToAbstract, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(numberingXML); err != nil {
		return nil, nil, err
	}
	root := doc.Root()
	if root == nil {
		return abstractNums{}, numToAbstract{}, nil
	}

	abstracts := abstractNums{}
	for _, an := range root.SelectElements("w:abstractNum") {
		aid := an.SelectAttrValue("w:abstractNumId", "")
		levels := map[string]levelMeta{}
		for _, lvl := range an.SelectElements("w:lvl") {
			ilvl := lvl.SelectAttrValue("w:ilvl", "")
			// Defaults apply only when the child element is missing.
			// Explicit empty val attrs (e.g. <w:lvlText w:val=""/>) are
			// preserved verbatim — they're how Word encodes "no label
			// at this level."
			start := 1
			if startEl := lvl.SelectElement("w:start"); startEl != nil {
				if n, err := strconv.Atoi(startEl.SelectAttrValue("w:val", "1")); err == nil {
					start = n
				}
			}
			numFmt := "decimal"
			if fmtEl := lvl.SelectElement("w:numFmt"); fmtEl != nil {
				numFmt = fmtEl.SelectAttrValue("w:val", "decimal")
			}
			lvlText := "%1."
			if lvlTextEl := lvl.SelectElement("w:lvlText"); lvlTextEl != nil {
				lvlText = lvlTextEl.SelectAttrValue("w:val", "")
			}
			levels[ilvl] = levelMeta{Start: start, NumFmt: numFmt, LvlText: lvlText}
		}
		abstracts[aid] = levels
	}

	resolve := numToAbstract{}
	for _, n := range root.SelectElements("w:num") {
		nid := n.SelectAttrValue("w:numId", "")
		if abn := n.SelectElement("w:abstractNumId"); abn != nil {
			resolve[nid] = abn.SelectAttrValue("w:val", "")
		}
	}
	return abstracts, resolve, nil
}

package docx2md

import (
	"regexp"
	"strings"

	"github.com/beevik/etree"
)

// styleInfo is the resolved metadata for a single styleId after walking
// the basedOn chain. NumID and Ilvl are empty strings if no style in the
// chain declares numPr; the Has* flags disambiguate "absent" from "set to
// empty string".
type styleInfo struct {
	Name         string
	BasedOn      string
	HeadingLevel int  // 1..9, or 0 if not a heading
	IsTitle      bool
	NumID        string
	Ilvl         string
	HasNumID     bool
}

// rawStyle is what we extract for one style before walking basedOn.
type rawStyle struct {
	Name     string
	BasedOn  string
	OwnNumID string
	OwnIlvl  string
	HasOwn   bool
}

var headingNameRE = regexp.MustCompile(`^heading\s*([1-9])$`)

// parseStyles parses the contents of word/styles.xml and returns a map
// keyed by styleId. Heading detection is by semantic name (case-insensitive
// "heading 1".."heading 9", "title"), resolved through the basedOn chain.
//
// The basedOn chain is walked iteratively (no recursion) with a seen-set
// for cycle detection and a depth-10 cap as belt-and-suspenders, matching
// the Python depth=10 cap.
func parseStyles(stylesXML []byte) (map[string]styleInfo, error) {
	if len(stylesXML) == 0 {
		return map[string]styleInfo{}, nil
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(stylesXML); err != nil {
		return nil, err
	}
	root := doc.Root()
	if root == nil {
		return map[string]styleInfo{}, nil
	}

	raw := map[string]rawStyle{}
	for _, s := range root.SelectElements("w:style") {
		sid := s.SelectAttrValue("w:styleId", "")
		if sid == "" {
			continue
		}
		name := ""
		if nameEl := s.SelectElement("w:name"); nameEl != nil {
			name = strings.TrimSpace(nameEl.SelectAttrValue("w:val", ""))
		}
		basedOn := ""
		if basedEl := s.SelectElement("w:basedOn"); basedEl != nil {
			basedOn = basedEl.SelectAttrValue("w:val", "")
		}
		var ownNumID, ownIlvl string
		hasOwn := false
		if pPr := s.SelectElement("w:pPr"); pPr != nil {
			if numPr := pPr.SelectElement("w:numPr"); numPr != nil {
				if nid := numPr.SelectElement("w:numId"); nid != nil {
					ownNumID = nid.SelectAttrValue("w:val", "")
					hasOwn = true
				}
				if il := numPr.SelectElement("w:ilvl"); il != nil {
					ownIlvl = il.SelectAttrValue("w:val", "")
				} else if hasOwn {
					ownIlvl = "0"
				}
			}
		}
		raw[sid] = rawStyle{
			Name:     name,
			BasedOn:  basedOn,
			OwnNumID: ownNumID,
			OwnIlvl:  ownIlvl,
			HasOwn:   hasOwn,
		}
	}

	out := make(map[string]styleInfo, len(raw))
	for sid, info := range raw {
		level, isTitle := resolveHeading(raw, sid)
		numID, ilvl, hasNum := resolveNum(raw, sid)
		out[sid] = styleInfo{
			Name:         info.Name,
			BasedOn:      info.BasedOn,
			HeadingLevel: level,
			IsTitle:      isTitle,
			NumID:        numID,
			Ilvl:         ilvl,
			HasNumID:     hasNum,
		}
	}
	return out, nil
}

// resolveHeading walks the basedOn chain iteratively until it finds a style
// whose semantic name is "heading N" or "title". Returns (level, isTitle).
// Cycles and missing parents both terminate cleanly.
func resolveHeading(raw map[string]rawStyle, sid string) (int, bool) {
	seen := map[string]bool{}
	cur := sid
	for depth := 0; depth <= 10; depth++ {
		if cur == "" || seen[cur] {
			return 0, false
		}
		info, ok := raw[cur]
		if !ok {
			return 0, false
		}
		seen[cur] = true
		name := strings.ToLower(info.Name)
		if m := headingNameRE.FindStringSubmatch(name); m != nil {
			n := int(m[1][0] - '0')
			return n, false
		}
		if name == "title" {
			return 0, true
		}
		cur = info.BasedOn
	}
	return 0, false
}

// resolveNum walks the basedOn chain iteratively until it finds a style
// declaring a numId/ilvl. Returns (numId, ilvl, hasNumID).
func resolveNum(raw map[string]rawStyle, sid string) (string, string, bool) {
	seen := map[string]bool{}
	cur := sid
	for depth := 0; depth <= 10; depth++ {
		if cur == "" || seen[cur] {
			return "", "", false
		}
		info, ok := raw[cur]
		if !ok {
			return "", "", false
		}
		seen[cur] = true
		if info.HasOwn {
			return info.OwnNumID, info.OwnIlvl, true
		}
		cur = info.BasedOn
	}
	return "", "", false
}

// findHeading1StyleID returns a styleId whose semantic name is "heading 1".
// Falls back to "Heading1" — Word's built-in style ID, which works even
// when not declared in styles.xml (matches the Python fallback).
func findHeading1StyleID(styleMap map[string]styleInfo) string {
	for sid, info := range styleMap {
		if info.HeadingLevel == 1 && strings.EqualFold(info.Name, "heading 1") {
			return sid
		}
	}
	return "Heading1"
}

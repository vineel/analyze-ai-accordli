package docx2md

import (
	"regexp"
	"strings"
)

// Compile once. (?m) puts ^/$ on line boundaries; the four passes are
// RE2-clean: no lookaround, no backreferences, lazy +? is supported.
var (
	reH1Heading      = regexp.MustCompile(`(?m)^# +(\d+)\. +(.+?)[ \t]*$`)
	reEscapedListNum = regexp.MustCompile(`(?m)^(\d+)\\\.\s+\*\*([^*]+?)\*\*\.?`)
	reBoldNum        = regexp.MustCompile(`(?m)^\*\*(\d+)\.\s+([^*]+?)\*\*\.?`)
	reEscapedParen   = regexp.MustCompile(`(?m)^\\\(([a-zA-Z0-9]+)\\\)`)
	rePeelBoldTitle  = regexp.MustCompile(`^\*\*(.+?)\*\*\.?\s*(.*)$`)
)

// NormalizeSectionHeadings forces every top-level section heading to
// `**N. Title.**`. Pandoc emits three different shapes for what are
// semantically the same construct:
//
//   - `# N. Title`         (h1 for paragraphs styled as headings)
//   - `N\. **Title**`      (escaped numbered-list item from injected labels)
//   - `**N. Title**`       (plain bold inline heading)
//
// All three are renormalized to one shape so downstream readers see a single
// consistent form. Subsection labels like `5.1` are not matched: the regexes
// require a single integer followed by a single period.
//
// A fourth pass unescapes `\(label\)` markers at line start (pandoc escapes
// these defensively; the contracts don't use the syntax that would require it).
func NormalizeSectionHeadings(md string) string {
	// Format A: H1 heading -> bold, peeling any inline body after the title.
	md = replaceWithGroups(reH1Heading, md, func(g []string) string {
		num, raw := g[1], g[2]
		title, body := peelBoldTitle(raw)
		if body != "" {
			return "**" + num + ". " + title + ".** " + body
		}
		return "**" + num + ". " + title + ".**"
	})

	// Format B: escaped numbered-list label `N\. **Title**` -> `**N. Title.**`.
	md = replaceWithGroups(reEscapedListNum, md, func(g []string) string {
		num, title := g[1], g[2]
		return "**" + num + ". " + strings.TrimRight(title, ".") + ".**"
	})

	// Format C: already `**N. Title**`, normalize whitespace and trailing period.
	md = replaceWithGroups(reBoldNum, md, func(g []string) string {
		num, title := g[1], g[2]
		title = strings.TrimRight(strings.TrimRight(title, "."), " \t")
		return "**" + num + ". " + title + ".**"
	})

	// Lettered/roman/numeric subsection markers at line start: unescape `\(N\)`.
	md = replaceWithGroups(reEscapedParen, md, func(g []string) string {
		return "(" + g[1] + ")"
	})

	return md
}

// peelBoldTitle splits a Format-A title cell into (clean_title, trailing_body).
// If raw starts with a bold span, the bold is the title and the rest is body;
// otherwise the whole cell is the title.
func peelBoldTitle(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if m := rePeelBoldTitle.FindStringSubmatch(raw); m != nil {
		return strings.TrimRight(m[1], ". \t"), m[2]
	}
	return strings.TrimRight(raw, ". \t"), ""
}

// replaceWithGroups runs fn on each non-overlapping match of re in s and
// returns the transformed string. fn receives the full submatch slice
// (g[0] = full match, g[1..] = capture groups).
//
// Implementation note: ReplaceAllStringFunc only exposes the matched
// substring. We re-run FindStringSubmatch on that substring to recover the
// groups, which is safe because anchored alternatives (`^`) still match at
// the start of the matched substring.
func replaceWithGroups(re *regexp.Regexp, s string, fn func(groups []string) string) string {
	return re.ReplaceAllStringFunc(s, func(match string) string {
		return fn(re.FindStringSubmatch(match))
	})
}

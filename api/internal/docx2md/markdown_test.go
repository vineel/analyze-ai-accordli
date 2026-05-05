package docx2md

import "testing"

func TestNormalizeSectionHeadingsFormatA_H1(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Plain H1 with title-only
		{"# 1. Definitions", "**1. Definitions.**"},
		// H1 with trailing whitespace
		{"# 2. Term and Termination   ", "**2. Term and Termination.**"},
		// H1 where title is itself wrapped in bold (peeled)
		{"# 3. **Confidentiality**", "**3. Confidentiality.**"},
		// H1 with trailing period in the bold title
		{"# 4. **Indemnity.**", "**4. Indemnity.**"},
		// H1 carrying body after a bold title
		{
			"# 5. **Payment**. Customer shall pay invoices within 30 days.",
			"**5. Payment.** Customer shall pay invoices within 30 days.",
		},
	}
	for _, c := range cases {
		got := NormalizeSectionHeadings(c.in)
		if got != c.want {
			t.Errorf("FormatA(%q):\n  got:  %q\n  want: %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeSectionHeadingsFormatB_EscapedListNum(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`1\. **Definitions**`, "**1. Definitions.**"},
		{`2\. **Term and Termination**.`, "**2. Term and Termination.**"},
		{`10\. **Miscellaneous**`, "**10. Miscellaneous.**"},
	}
	for _, c := range cases {
		got := NormalizeSectionHeadings(c.in)
		if got != c.want {
			t.Errorf("FormatB(%q):\n  got:  %q\n  want: %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeSectionHeadingsFormatC_BoldNum(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"**1. Definitions**", "**1. Definitions.**"},
		{"**2. Term and Termination.**", "**2. Term and Termination.**"},
		{"**3. Indemnity.**", "**3. Indemnity.**"},
	}
	for _, c := range cases {
		got := NormalizeSectionHeadings(c.in)
		if got != c.want {
			t.Errorf("FormatC(%q):\n  got:  %q\n  want: %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeSectionHeadings_DoesNotMatchSubsections(t *testing.T) {
	// Subsection labels (N.M) must not be renormalized.
	in := "**5.1 Sub-thing**"
	got := NormalizeSectionHeadings(in)
	if got != in {
		t.Errorf("subsection should be unchanged:\n  got:  %q\n  want: %q", got, in)
	}

	in2 := `5\.1 thing` // not a Format-B match either
	got2 := NormalizeSectionHeadings(in2)
	if got2 != in2 {
		t.Errorf("subsection should be unchanged:\n  got:  %q\n  want: %q", got2, in2)
	}
}

func TestNormalizeSectionHeadingsEscapedParen(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`\(a\) text`, "(a) text"},
		{`\(iv\) some text`, "(iv) some text"},
		{`\(2\) third item`, "(2) third item"},
		// Mid-line should NOT be unescaped (regex anchored at line start).
		{`prefix \(a\) inline`, `prefix \(a\) inline`},
	}
	for _, c := range cases {
		got := NormalizeSectionHeadings(c.in)
		if got != c.want {
			t.Errorf("EscapedParen(%q):\n  got:  %q\n  want: %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeSectionHeadingsMultiline(t *testing.T) {
	in := "# 1. Definitions\nIntro paragraph.\n\n# 2. **Term**.\n" +
		`3\. **Payment**` + "\n" +
		"**4. Confidentiality**\n" +
		`\(a\) sub-item`
	want := "**1. Definitions.**\nIntro paragraph.\n\n**2. Term.**\n" +
		"**3. Payment.**\n" +
		"**4. Confidentiality.**\n" +
		"(a) sub-item"
	got := NormalizeSectionHeadings(in)
	if got != want {
		t.Errorf("multiline:\n  got:  %q\n  want: %q", got, want)
	}
}

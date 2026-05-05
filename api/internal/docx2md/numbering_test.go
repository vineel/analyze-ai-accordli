package docx2md

import "testing"

func TestAlpha(t *testing.T) {
	cases := []struct {
		n     int
		lower bool
		want  string
	}{
		{0, true, ""},
		{-1, true, ""},
		{1, true, "a"},
		{2, true, "b"},
		{26, true, "z"},
		{27, true, "aa"},
		{28, true, "ab"},
		{52, true, "az"},
		{53, true, "ba"},
		{1, false, "A"},
		{26, false, "Z"},
		{27, false, "AA"},
	}
	for _, c := range cases {
		got := alpha(c.n, c.lower)
		if got != c.want {
			t.Errorf("alpha(%d, %v) = %q, want %q", c.n, c.lower, got, c.want)
		}
	}
}

func TestRoman(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, ""},
		{-3, ""},
		{1, "I"},
		{4, "IV"},
		{5, "V"},
		{9, "IX"},
		{10, "X"},
		{40, "XL"},
		{49, "XLIX"},
		{90, "XC"},
		{400, "CD"},
		{900, "CM"},
		{1994, "MCMXCIV"},
	}
	for _, c := range cases {
		got := roman(c.n)
		if got != c.want {
			t.Errorf("roman(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestFmtNumber(t *testing.T) {
	cases := []struct {
		n      int
		fmt    string
		want   string
	}{
		{1, "decimal", "1"},
		{42, "decimal", "42"},
		{1, "lowerLetter", "a"},
		{27, "lowerLetter", "aa"},
		{3, "upperLetter", "C"},
		{4, "lowerRoman", "iv"},
		{5, "upperRoman", "V"},
		{1, "bullet", ""},
		{1, "none", ""},
		{1, "garbage", "1"}, // unknown -> decimal fallback
	}
	for _, c := range cases {
		got := fmtNumber(c.n, c.fmt)
		if got != c.want {
			t.Errorf("fmtNumber(%d, %q) = %q, want %q", c.n, c.fmt, got, c.want)
		}
	}
}

func TestRenderLabel(t *testing.T) {
	meta := map[string]levelMeta{
		"0": {Start: 1, NumFmt: "decimal", LvlText: "%1."},
		"1": {Start: 1, NumFmt: "decimal", LvlText: "%1.%2"},
		"2": {Start: 1, NumFmt: "lowerLetter", LvlText: "(%3)"},
		"3": {Start: 1, NumFmt: "lowerRoman", LvlText: "(%4)"},
	}
	cases := []struct {
		lvlText  string
		counters map[string]int
		want     string
	}{
		// Top-level decimal label
		{"%1.", map[string]int{"0": 5}, "5."},
		// Nested 2-level
		{"%1.%2", map[string]int{"0": 3, "1": 7}, "3.7"},
		// Lettered with counter at level 2
		{"(%3)", map[string]int{"2": 2}, "(b)"},
		// Roman level 3
		{"(%4)", map[string]int{"3": 4}, "(iv)"},
		// Token referring to a level with no meta -> decimal fallback
		{"%5.", map[string]int{"4": 8}, "8."},
		// Missing counter -> 0 rendered (decimal: "0", lowerLetter: "")
		{"%1", map[string]int{}, "0"},
		// No tokens -> unchanged
		{"see above", map[string]int{}, "see above"},
	}
	for _, c := range cases {
		got := renderLabel(c.lvlText, meta, c.counters)
		if got != c.want {
			t.Errorf("renderLabel(%q, ..., %v) = %q, want %q", c.lvlText, c.counters, got, c.want)
		}
	}
}

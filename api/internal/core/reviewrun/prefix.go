package reviewrun

import (
	"fmt"
	"strings"
)

// PrefixSystem is the system prompt every ReviewRun uses. Stable text
// — versioning is implicit in the Run's prefix bytes.
const PrefixSystem = `You are an experienced transactional lawyer reviewing an agreement on
behalf of a client. You will read the agreement once, then answer a
series of structured questions about it. Be precise, ground every
statement in the agreement text, and never invent facts.

Some prompts will ask for JSONL output (one JSON object per line) and
others for prose. The instructions for each are in the user message
that follows the agreement. Output only what is asked for — no
preamble, no markdown fences, no commentary.`

// MatterMetadata is the small shaped block prepended to the contract
// markdown. Today it's just the title; later phases add user answers
// and supplemental docs.
type MatterMetadata struct {
	Title string
}

// BuildPrefix assembles the shared user-message block. This is the
// content sent with cache_control: ephemeral on the first call of a
// Run; subsequent Lens / summary calls reuse the bytes verbatim and
// hit Anthropic's prompt cache.
func BuildPrefix(meta MatterMetadata, markdown string) string {
	var b strings.Builder
	b.WriteString("## Matter metadata\n\n")
	fmt.Fprintf(&b, "- Title: %s\n", meta.Title)
	b.WriteString("\n## Agreement (markdown)\n\n")
	b.WriteString(markdown)
	if !strings.HasSuffix(markdown, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

// EstimateTokens returns a rough token count for storing on the Run
// row. ~4 chars/token is the standard back-of-envelope ratio.
func EstimateTokens(s string) int {
	return len(s) / 4
}

package docx2md

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// runPandoc shells `pandoc -f docx -t gfm <srcDocx>` and returns the
// markdown bytes from stdout. Errors include pandoc's stderr to make
// failures actionable.
func runPandoc(ctx context.Context, srcDocx string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "pandoc", "-f", "docx", "-t", "gfm", srcDocx)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pandoc failed: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

package reviewrun

import "os"

// readFile is in its own file to keep the orchestrator's imports
// focused. Phase Blob replaces this with an HTTP fetch behind a SAS.
func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

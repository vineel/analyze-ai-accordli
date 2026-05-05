package httpapi

import "io"

func copyAll(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }

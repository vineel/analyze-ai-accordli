// Package storage is the blob-store seam.
//
// SoloMocky:   LocalFSBlob writing to ./var/blob/.
// Phase Blob:  AzureBlob replaces it.
package storage

import (
	"context"
	"io"
)

type BlobStore interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	SignedURL(ctx context.Context, key string, ttlSeconds int) (string, error)
}

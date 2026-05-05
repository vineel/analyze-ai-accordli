package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LocalFSBlob struct {
	root string
}

func NewLocalFS(root string) *LocalFSBlob { return &LocalFSBlob{root: root} }

func (b *LocalFSBlob) path(key string) string { return filepath.Join(b.root, key) }

func (b *LocalFSBlob) Put(_ context.Context, key string, r io.Reader) error {
	p := b.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (b *LocalFSBlob) Get(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(b.path(key))
}

func (b *LocalFSBlob) SignedURL(_ context.Context, key string, _ int) (string, error) {
	// Local FS has no URL; return a file:// URL so callers can detect.
	abs, err := filepath.Abs(b.path(key))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("file://%s", abs), nil
}

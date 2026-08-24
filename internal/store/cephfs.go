package store

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

// CephFSConfig points at a mounted CephFS directory (Rook volumeMount or ceph-fuse).
type CephFSConfig struct {
	Root   string
	Prefix string
}

// CephFSStore implements StorageProvider against a local CephFS mount (no CGO).
type CephFSStore struct {
	root   string
	prefix string
}

func NewCephFS(cfg CephFSConfig) (*CephFSStore, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		return nil, fmt.Errorf("cephfs root is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("cephfs root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("cephfs root %q is not a directory", root)
	}
	return &CephFSStore{
		root:   filepath.Clean(root),
		prefix: strings.Trim(cfg.Prefix, "/"),
	}, nil
}

func (s *CephFSStore) abs(key string) (string, error) {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if key == "" || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid key %q", key)
	}
	rel := key
	if s.prefix != "" {
		rel = s.prefix + "/" + key
	}
	abs := filepath.Join(s.root, filepath.FromSlash(rel))
	cleanRoot := filepath.Clean(s.root) + string(os.PathSeparator)
	if !strings.HasPrefix(abs+string(os.PathSeparator), cleanRoot) && abs != filepath.Clean(s.root) {
		return "", fmt.Errorf("invalid key %q", key)
	}
	return abs, nil
}

func (s *CephFSStore) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_, err := os.Stat(s.root)
	return err
}

func (s *CephFSStore) EnsureBucket(context.Context) error { return nil }

func (s *CephFSStore) Put(ctx context.Context, key string, data io.Reader, _ int64) error {
	abs, err := s.abs(key)
	if err != nil {
		return err
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(abs), 0o750); mkdirErr != nil {
		return mkdirErr
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // abs is canonicalized and confined under root by abs()
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, data)
	return err
}

func (s *CephFSStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	abs, err := s.abs(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(abs) //nolint:gosec // abs is canonicalized and confined under root by abs()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, contracts.ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

func (s *CephFSStore) Delete(ctx context.Context, key string) error {
	abs, err := s.abs(key)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

func (s *CephFSStore) Move(ctx context.Context, src, dst string) error {
	srcAbs, err := s.abs(src)
	if err != nil {
		return err
	}
	dstAbs, err := s.abs(dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstAbs), 0o750); err != nil {
		return err
	}
	if err := os.Rename(srcAbs, dstAbs); err != nil {
		if os.IsNotExist(err) {
			return contracts.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *CephFSStore) Exists(ctx context.Context, key string) (bool, error) {
	abs, err := s.abs(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(abs)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *CephFSStore) Stat(ctx context.Context, key string) (contracts.ObjectInfo, error) {
	abs, err := s.abs(key)
	if err != nil {
		return contracts.ObjectInfo{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return contracts.ObjectInfo{}, contracts.ErrNotFound
		}
		return contracts.ObjectInfo{}, err
	}
	if info.IsDir() {
		return contracts.ObjectInfo{}, contracts.ErrNotFound
	}
	return contracts.ObjectInfo{
		Key:          key,
		Size:         info.Size(),
		ContentType:  "application/octet-stream",
		LastModified: info.ModTime(),
	}, nil
}

func (s *CephFSStore) List(ctx context.Context, prefix string) ([]contracts.ObjectInfo, error) {
	base := s.root
	if s.prefix != "" {
		base = filepath.Join(s.root, filepath.FromSlash(s.prefix))
	}
	trim := strings.Trim(prefix, "/")
	if trim != "" {
		base = filepath.Join(base, filepath.FromSlash(trim))
	}
	var out []contracts.ObjectInfo
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if s.prefix != "" {
			key = strings.TrimPrefix(key, s.prefix+"/")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, contracts.ObjectInfo{
			Key:          key,
			Size:         info.Size(),
			ContentType:  "application/octet-stream",
			LastModified: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *CephFSStore) Stream(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	rc, err := s.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		seeker, ok := rc.(io.ReadSeeker)
		if !ok {
			_ = rc.Close()
			return nil, fmt.Errorf("cephfs stream: cannot seek")
		}
		if _, err := seeker.Seek(offset, io.SeekStart); err != nil {
			_ = rc.Close()
			return nil, err
		}
	}
	if length > 0 {
		return io.NopCloser(io.LimitReader(rc, length)), nil
	}
	return rc, nil
}

var (
	_ Backend                   = (*CephFSStore)(nil)
	_ contracts.StorageProvider = (*CephFSStore)(nil)
	_ contracts.Streamable      = (*CephFSStore)(nil)
)

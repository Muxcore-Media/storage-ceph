//go:build ceph

package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/ceph/go-ceph/rados"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

// RADOSStore implements StorageProvider via librados (build tag ceph).
type RADOSStore struct {
	mu     sync.Mutex
	conn   *rados.Conn
	ioctx  *rados.IOContext
	pool   string
	prefix string
}

func NewRADOS(cfg RADOSConfig) (Backend, error) {
	if strings.TrimSpace(cfg.Pool) == "" {
		return nil, fmt.Errorf("rados pool is required")
	}
	conn, err := rados.NewConnWithUser(cfg.User)
	if err != nil {
		return nil, fmt.Errorf("rados conn: %w", err)
	}
	for _, mon := range strings.Split(cfg.Monitors, ",") {
		mon = strings.TrimSpace(mon)
		if mon == "" {
			continue
		}
		if err := conn.AddMonitor(mon); err != nil {
			return nil, fmt.Errorf("add monitor %q: %w", mon, err)
		}
	}
	if cfg.Keyring != "" {
		if err := conn.ReadConfigFile(cfg.Keyring); err != nil {
			if err := conn.SetConfigOption("keyring", cfg.Keyring); err != nil {
				return nil, fmt.Errorf("keyring: %w", err)
			}
		}
	}
	if err := conn.Connect(); err != nil {
		return nil, fmt.Errorf("rados connect: %w", err)
	}
	ioctx, err := conn.OpenIOContext(cfg.Pool)
	if err != nil {
		conn.Shutdown()
		return nil, fmt.Errorf("open pool %q: %w", cfg.Pool, err)
	}
	return &RADOSStore{
		conn:   conn,
		ioctx:  ioctx,
		pool:   cfg.Pool,
		prefix: strings.Trim(cfg.Prefix, "/"),
	}, nil
}

func (s *RADOSStore) objKey(key string) string {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

func (s *RADOSStore) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ioctx == nil {
		return fmt.Errorf("rados not connected")
	}
	return nil
}

func (s *RADOSStore) EnsureBucket(context.Context) error { return nil }

func (s *RADOSStore) Put(ctx context.Context, key string, data io.Reader, size int64) error {
	body, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ioctx.WriteFull(s.objKey(key), body)
}

func (s *RADOSStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.ioctx.Read(s.objKey(key))
	if err != nil {
		if rados.ErrNotFound == err {
			return nil, contracts.ErrNotFound
		}
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (s *RADOSStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.ioctx.Delete(s.objKey(key))
	if err != nil && rados.ErrNotFound == err {
		return nil
	}
	return err
}

func (s *RADOSStore) Move(ctx context.Context, src, dst string) error {
	rc, err := s.Get(ctx, src)
	if err != nil {
		return err
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	if err := s.Put(ctx, dst, bytes.NewReader(body), int64(len(body))); err != nil {
		return err
	}
	return s.Delete(ctx, src)
}

func (s *RADOSStore) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Stat(ctx, key)
	if err == nil {
		return true, nil
	}
	if err == contracts.ErrNotFound {
		return false, nil
	}
	return false, err
}

func (s *RADOSStore) Stat(ctx context.Context, key string) (contracts.ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stat, err := s.ioctx.Stat(s.objKey(key))
	if err != nil {
		if rados.ErrNotFound == err {
			return contracts.ObjectInfo{}, contracts.ErrNotFound
		}
		return contracts.ObjectInfo{}, err
	}
	return contracts.ObjectInfo{
		Key:         key,
		Size:        int64(stat.Size),
		ContentType: "application/octet-stream",
	}, nil
}

func (s *RADOSStore) List(ctx context.Context, prefix string) ([]contracts.ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	iter, err := s.ioctx.Iter()
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	want := s.objKey(strings.TrimPrefix(prefix, "/"))
	var out []contracts.ObjectInfo
	for iter.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		name := iter.Value().Name
		if !strings.HasPrefix(name, want) {
			continue
		}
		key := name
		if s.prefix != "" {
			key = strings.TrimPrefix(key, s.prefix+"/")
		}
		stat, err := s.ioctx.Stat(name)
		if err != nil {
			continue
		}
		out = append(out, contracts.ObjectInfo{
			Key:  key,
			Size: int64(stat.Size),
		})
	}
	return out, iter.Err()
}

func (s *RADOSStore) Stream(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	rc, err := s.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := io.CopyN(io.Discard, rc, offset); err != nil {
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
	_ Backend                   = (*RADOSStore)(nil)
	_ contracts.StorageProvider = (*RADOSStore)(nil)
	_ contracts.Streamable      = (*RADOSStore)(nil)
)

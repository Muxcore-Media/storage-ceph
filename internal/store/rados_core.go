package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

const radosChunkSize = 4 << 20 // 4 MiB

type radosObjectStat struct {
	Size uint64
}

type radosIterator interface {
	Next() bool
	Value() string
	Err() error
	Close()
}

type radosIO interface {
	Write(name string, data []byte, offset uint64) error
	Read(name string, offset, length uint64) ([]byte, error)
	Delete(name string) error
	Stat(name string) (radosObjectStat, error)
	Iter() (radosIterator, error)
	Ping(ctx context.Context) error
	Close() error
}

type radosStore struct {
	io     radosIO
	prefix string
	mu     sync.Mutex
}

// sizeToInt64 converts a RADOS object size to int64, clamping at MaxInt64.
func sizeToInt64(n uint64) int64 {
	if n > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(n)
}

func newRADOSStore(rio radosIO, prefix string) *radosStore {
	return &radosStore{io: rio, prefix: strings.Trim(prefix, "/")}
}

func (s *radosStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.io == nil {
		return nil
	}
	err := s.io.Close()
	s.io = nil
	return err
}

func (s *radosStore) objKey(key string) string {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

func (s *radosStore) withIO() (radosIO, error) {
	s.mu.Lock()
	ioctx := s.io
	s.mu.Unlock()
	if ioctx == nil {
		return nil, fmt.Errorf("rados not connected")
	}
	return ioctx, nil
}

func (s *radosStore) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	ioctx, err := s.withIO()
	if err != nil {
		return err
	}
	return ioctx.Ping(ctx)
}

func (s *radosStore) EnsureBucket(context.Context) error { return nil }

func (s *radosStore) Put(_ context.Context, key string, data io.Reader, _ int64) error {
	ioctx, err := s.withIO()
	if err != nil {
		return err
	}
	obj := s.objKey(key)
	buf := make([]byte, radosChunkSize)
	var offset uint64
	for {
		n, readErr := data.Read(buf)
		if n > 0 {
			if werr := ioctx.Write(obj, buf[:n], offset); werr != nil {
				return werr
			}
			offset += uint64(n)
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (s *radosStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.Stream(ctx, key, 0, 0)
}

func (s *radosStore) Delete(ctx context.Context, key string) error {
	ioctx, err := s.withIO()
	if err != nil {
		return err
	}
	return ioctx.Delete(s.objKey(key))
}

func (s *radosStore) Move(ctx context.Context, src, dst string) error {
	rc, err := s.Get(ctx, src)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	if err := s.Put(ctx, dst, rc, -1); err != nil {
		return err
	}
	return s.Delete(ctx, src)
}

func (s *radosStore) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Stat(ctx, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, contracts.ErrNotFound) {
		return false, nil
	}
	return false, err
}

func (s *radosStore) Stat(_ context.Context, key string) (contracts.ObjectInfo, error) {
	ioctx, err := s.withIO()
	if err != nil {
		return contracts.ObjectInfo{}, err
	}
	stat, err := ioctx.Stat(s.objKey(key))
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			return contracts.ObjectInfo{}, contracts.ErrNotFound
		}
		return contracts.ObjectInfo{}, err
	}
	return contracts.ObjectInfo{
		Key:         key,
		Size:        sizeToInt64(stat.Size),
		ContentType: "application/octet-stream",
	}, nil
}

func (s *radosStore) List(ctx context.Context, prefix string) ([]contracts.ObjectInfo, error) {
	ioctx, err := s.withIO()
	if err != nil {
		return nil, err
	}
	iter, err := ioctx.Iter()
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
		name := iter.Value()
		if want != "" && !strings.HasPrefix(name, want) {
			continue
		}
		key := name
		if s.prefix != "" {
			key = strings.TrimPrefix(key, s.prefix+"/")
		}
		stat, statErr := ioctx.Stat(name)
		if statErr != nil {
			continue
		}
		out = append(out, contracts.ObjectInfo{
			Key:  key,
			Size: sizeToInt64(stat.Size),
		})
	}
	return out, iter.Err()
}

func (s *radosStore) Stream(_ context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	ioctx, err := s.withIO()
	if err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, fmt.Errorf("rados stream: negative offset %d", offset)
	}
	obj := s.objKey(key)
	stat, err := ioctx.Stat(obj)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			return nil, contracts.ErrNotFound
		}
		return nil, err
	}
	off := uint64(offset) // offset >= 0 checked above
	end := stat.Size
	if length > 0 {
		// Clamp rather than add, so a huge length cannot overflow uint64.
		if remaining := stat.Size - min(off, stat.Size); uint64(length) < remaining {
			end = off + uint64(length)
		}
	}
	if off >= stat.Size {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return &radosStreamReader{
		io:     ioctx,
		key:    obj,
		offset: off,
		end:    end,
	}, nil
}

type radosStreamReader struct {
	io     radosIO
	key    string
	buf    []byte
	offset uint64
	end    uint64
	bufOff int
	closed bool
}

func (r *radosStreamReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.EOF
	}
	if r.offset >= r.end {
		return 0, io.EOF
	}
	if len(r.buf) == 0 || r.bufOff >= len(r.buf) {
		chunk := uint64(radosChunkSize)
		if remain := r.end - r.offset; remain < chunk {
			chunk = remain
		}
		b, err := r.io.Read(r.key, r.offset, chunk)
		if err != nil {
			return 0, err
		}
		r.buf = b
		r.bufOff = 0
	}
	n := copy(p, r.buf[r.bufOff:])
	r.bufOff += n
	r.offset += uint64(n) //nolint:gosec // n comes from copy(), never negative
	if r.offset >= r.end {
		if n < len(p) {
			return n, io.EOF
		}
	}
	return n, nil
}

func (r *radosStreamReader) Close() error {
	r.closed = true
	r.buf = nil
	return nil
}

var (
	_ Backend                   = (*radosStore)(nil)
	_ contracts.StorageProvider = (*radosStore)(nil)
	_ contracts.Streamable      = (*radosStore)(nil)
)

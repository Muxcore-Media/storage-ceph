package store

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

type fakeRadosIO struct {
	mu      sync.Mutex
	objects map[string][]byte
	closed  bool
}

func newFakeRadosIO() *fakeRadosIO {
	return &fakeRadosIO{objects: make(map[string][]byte)}
}

func (f *fakeRadosIO) Write(name string, data []byte, offset uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur := f.objects[name]
	if offset > uint64(len(cur)) {
		padding := make([]byte, offset-uint64(len(cur)))
		cur = append(cur, padding...)
	}
	end := offset + uint64(len(data))
	if end > uint64(len(cur)) {
		next := make([]byte, end)
		copy(next, cur)
		cur = next
	}
	copy(cur[offset:], data)
	f.objects[name] = cur
	return nil
}

func (f *fakeRadosIO) Read(name string, offset, length uint64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.objects[name]
	if !ok {
		return nil, contracts.ErrNotFound
	}
	if offset >= uint64(len(cur)) {
		return nil, nil
	}
	end := offset + length
	if end > uint64(len(cur)) {
		end = uint64(len(cur))
	}
	return append([]byte(nil), cur[offset:end]...), nil
}

func (f *fakeRadosIO) Delete(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, name)
	return nil
}

func (f *fakeRadosIO) Stat(name string) (radosObjectStat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.objects[name]
	if !ok {
		return radosObjectStat{}, contracts.ErrNotFound
	}
	return radosObjectStat{Size: uint64(len(cur))}, nil
}

func (f *fakeRadosIO) Iter() (radosIterator, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]string, 0, len(f.objects))
	for k := range f.objects {
		names = append(names, k)
	}
	return &fakeRadosIter{names: names}, nil
}

func (f *fakeRadosIO) Ping(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if f.closed {
		return io.EOF
	}
	return nil
}

func (f *fakeRadosIO) Close() error {
	f.closed = true
	return nil
}

type fakeRadosIter struct {
	names []string
	idx   int
}

func (i *fakeRadosIter) Next() bool {
	if i.idx >= len(i.names) {
		return false
	}
	i.idx++
	return true
}

func (i *fakeRadosIter) Value() string { return i.names[i.idx-1] }

func (i *fakeRadosIter) Err() error { return nil }

func (i *fakeRadosIter) Close() {}

func TestRADOSStoreChunkedPutGetStream(t *testing.T) {
	ioctx := newFakeRadosIO()
	st := newRADOSStore(ioctx, "data")
	ctx := context.Background()
	payload := bytes.Repeat([]byte("x"), 5<<20)
	if err := st.Put(ctx, "big.bin", bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rc, err := st.Stream(ctx, "big.bin", 100, 8)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload[100:108]) {
		t.Fatalf("range mismatch len=%d", len(got))
	}
	list, err := st.List(ctx, "")
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %v len=%d", err, len(list))
	}
	if err := st.Delete(ctx, "big.bin"); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if !ioctx.closed {
		t.Fatal("expected io closed")
	}
}

func TestRADOSHealthPing(t *testing.T) {
	ioctx := newFakeRadosIO()
	st := newRADOSStore(ioctx, "")
	if err := st.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	if err := st.Health(context.Background()); err == nil {
		t.Fatal("expected health failure after close")
	}
}

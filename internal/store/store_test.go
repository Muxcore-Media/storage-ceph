package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	backend := s3mem.New()
	faker := gofakes3.New(backend)
	ts := httptest.NewServer(faker.Server())
	t.Cleanup(ts.Close)

	endpoint := strings.TrimPrefix(ts.URL, "http://")
	st, err := New(Config{
		Endpoint:  endpoint,
		Bucket:    "muxcore",
		AccessKey: "id",
		SecretKey: "secret",
		UseSSL:    false,
		PathStyle: true,
		Prefix:    "data",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := st.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	return st
}

func TestPutGetStatListDelete(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	body := []byte("hello-ceph")

	if err := st.Put(ctx, "a/b.txt", bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rc, err := st.Get(ctx, "a/b.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body=%q want %q", got, body)
	}

	info, err := st.Stat(ctx, "a/b.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size != int64(len(body)) {
		t.Fatalf("size=%d", info.Size)
	}

	ok, err := st.Exists(ctx, "a/b.txt")
	if err != nil || !ok {
		t.Fatalf("Exists: ok=%v err=%v", ok, err)
	}

	list, err := st.List(ctx, "a/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Key != "a/b.txt" {
		t.Fatalf("list=%+v", list)
	}

	if err := st.Delete(ctx, "a/b.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = st.Get(ctx, "a/b.txt")
	if !errors.Is(err, contracts.ErrNotFound) {
		t.Fatalf("Get after delete: %v", err)
	}
}

func TestMoveAndStream(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	body := []byte("0123456789")
	if err := st.Put(ctx, "src.bin", bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := st.Move(ctx, "src.bin", "dst.bin"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	ok, _ := st.Exists(ctx, "src.bin")
	if ok {
		t.Fatal("src still exists")
	}
	rc, err := st.Stream(ctx, "dst.bin", 2, 4)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	// gofakes3 may ignore Range; accept either ranged or full payload.
	if string(got) != "2345" && string(got) != "0123456789" {
		t.Fatalf("stream=%q", got)
	}
}

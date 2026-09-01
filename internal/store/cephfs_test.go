package store

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func testCephFS(t *testing.T) *CephFSStore {
	t.Helper()
	root := filepath.Join(t.TempDir(), "cephfs")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := NewCephFS(CephFSConfig{Root: root, Prefix: "data"})
	if err != nil {
		t.Fatalf("NewCephFS: %v", err)
	}
	return st
}

func TestCephFSPutGetListDelete(t *testing.T) {
	st := testCephFS(t)
	ctx := context.Background()
	body := []byte("cephfs-payload")
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
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body=%q", got)
	}
	list, err := st.List(ctx, "a/")
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %v len=%d", err, len(list))
	}
	if err := st.Delete(ctx, "a/b.txt"); err != nil {
		t.Fatal(err)
	}
	_, err = st.Get(ctx, "a/b.txt")
	if err != contracts.ErrNotFound {
		t.Fatalf("Get after delete: %v", err)
	}
}

func TestCephFSListRejectsTraversal(t *testing.T) {
	st := testCephFS(t)
	ctx := context.Background()
	if _, err := st.List(ctx, "../"); err == nil {
		t.Fatal("expected error for ../ prefix")
	}
	if _, err := st.List(ctx, "foo/../../etc"); err == nil {
		t.Fatal("expected error for nested traversal prefix")
	}
}

func TestCephFSListRejectsFilePrefix(t *testing.T) {
	st := testCephFS(t)
	ctx := context.Background()
	body := []byte("file-prefix")
	if err := st.Put(ctx, "leaf.txt", bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatal(err)
	}
	if _, err := st.List(ctx, "leaf.txt"); err == nil {
		t.Fatal("expected error when prefix is a file")
	}
}

func TestCephFSStreamCloseReleasesFD(t *testing.T) {
	st := testCephFS(t)
	ctx := context.Background()
	body := []byte("0123456789abcdef")
	if err := st.Put(ctx, "stream.bin", bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatal(err)
	}
	rc, err := st.Stream(ctx, "stream.bin", 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "2345" {
		t.Fatalf("range=%q", got)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	rc2, err := st.Stream(ctx, "stream.bin", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = rc2.Close()
}

func TestOpenBackends(t *testing.T) {
	root := t.TempDir()
	fs, err := Open(ModuleConfig{Backend: "cephfs", CephFSRoot: root})
	if err != nil {
		t.Fatalf("cephfs open: %v", err)
	}
	if err := fs.Health(context.Background()); err != nil {
		t.Fatalf("health: %v", err)
	}
	_, err = Open(ModuleConfig{Backend: "rados", Pool: "muxcore"})
	if err == nil {
		t.Fatal("expected rados open error without ceph build tag")
	}
}

package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"testing"
	"time"
)

func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

func freePort(t *testing.T) int {
	t.Helper()
	var lc net.ListenConfig
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer lis.Close()
	return lis.Addr().(*net.TCPAddr).Port
}

func assertPutGetListDelete(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	body := []byte("hello-ceph-minio")

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
}

// TestMinIO_PutGetListDelete treats MinIO as a Ceph RGW stand-in on the laptop.
// Skips when Docker is unavailable — gofakes3 covers the default path.
func TestMinIO_PutGetListDelete(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("docker not available; gofakes3 httptest tests cover Put/Get/List/Delete (MinIO = RGW stand-in)")
	}

	port := freePort(t)
	name := fmt.Sprintf("storage-ceph-minio-%d", port)
	access, secret := "minioadmin", "minioadmin"

	run := exec.Command("docker", "run", "-d", "--rm",
		"--name", name,
		"-p", fmt.Sprintf("127.0.0.1:%d:9000", port),
		"-e", "MINIO_ROOT_USER="+access,
		"-e", "MINIO_ROOT_PASSWORD="+secret,
		"minio/minio:latest", "server", "/data",
	)
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("docker run minio: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "stop", "-t", "1", name).Run()
	})

	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(60 * time.Second)
	var st *Store
	for {
		st, err = New(Config{
			Endpoint:  endpoint,
			Bucket:    "muxcore",
			AccessKey: access,
			SecretKey: secret,
			UseSSL:    false,
			PathStyle: true,
			Prefix:    "data",
		})
		if err == nil {
			err = st.EnsureBucket(context.Background())
		}
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("minio not ready at %s: %v", endpoint, err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	assertPutGetListDelete(t, st)

	ctx := context.Background()
	body := []byte("0123456789")
	if err := st.Put(ctx, "range.bin", bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("Put range.bin: %v", err)
	}
	rc, err := st.Stream(ctx, "range.bin", 2, 4)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "2345" {
		t.Fatalf("minio stream=%q want 2345", got)
	}
}

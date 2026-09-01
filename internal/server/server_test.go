package server_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	storagev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/storage/v1"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"

	"github.com/Muxcore-Media/storage-ceph/internal/server"
	"github.com/Muxcore-Media/storage-ceph/internal/store"
)

func TestCapabilityStorageDialSmoke(t *testing.T) {
	backend := s3mem.New()
	faker := gofakes3.New(backend)
	ts := httptest.NewServer(faker.Server())
	t.Cleanup(ts.Close)

	st, err := store.New(store.Config{
		Endpoint:  strings.TrimPrefix(ts.URL, "http://"),
		Bucket:    "muxcore",
		AccessKey: "id",
		SecretKey: "secret",
		UseSSL:    false,
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}

	var lc net.ListenConfig
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	grpcSrv := grpc.NewServer()
	server.Register(grpcSrv, st)
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(grpcSrv.GracefulStop)

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := storagev1.NewStorageServiceClient(conn)
	ctx := context.Background()

	caps, err := client.Capabilities(ctx, &storagev1.CapabilitiesRequest{})
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	found := false
	for _, c := range caps.GetCapabilities() {
		if c == "streamable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("capabilities=%v want streamable", caps.GetCapabilities())
	}

	body := []byte("dial-smoke")
	put, err := client.Put(ctx)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := put.Send(&storagev1.PutRequest{
		Key:       "smoke/obj.bin",
		TotalSize: int64(len(body)),
		Chunk:     body,
	}); err != nil {
		t.Fatalf("Put.Send: %v", err)
	}
	if _, err := put.CloseAndRecv(); err != nil {
		t.Fatalf("Put.CloseAndRecv: %v", err)
	}

	stat, err := client.Stat(ctx, &storagev1.StatRequest{Key: "smoke/obj.bin"})
	if err != nil || !stat.GetFound() {
		t.Fatalf("Stat: %v found=%v", err, stat.GetFound())
	}

	list, err := client.List(ctx, &storagev1.ListRequest{Prefix: "smoke/"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.GetObjects()) != 1 || list.GetObjects()[0].GetKey() != "smoke/obj.bin" {
		t.Fatalf("list=%+v", list.GetObjects())
	}

	get, err := client.Get(ctx, &storagev1.GetRequest{Key: "smoke/obj.bin"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var got []byte
	for {
		msg, recvErr := get.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			t.Fatalf("Get.Recv: %v", recvErr)
		}
		got = append(got, msg.GetChunk()...)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("get body=%q", got)
	}

	del, err := client.Delete(ctx, &storagev1.DeleteRequest{Key: "smoke/obj.bin"})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !del.GetDeleted() {
		t.Fatal("expected deleted=true")
	}
}

type streamingPutStore struct {
	store.Backend
	started chan struct{}
}

func (s *streamingPutStore) Put(ctx context.Context, key string, data io.Reader, size int64) error {
	close(s.started)
	return s.Backend.Put(ctx, key, data, size)
}

func TestPutStreamsBeforeAllChunksReceived(t *testing.T) {
	backend := s3mem.New()
	faker := gofakes3.New(backend)
	ts := httptest.NewServer(faker.Server())
	t.Cleanup(ts.Close)

	base, err := store.New(store.Config{
		Endpoint:  strings.TrimPrefix(ts.URL, "http://"),
		Bucket:    "muxcore",
		AccessKey: "id",
		SecretKey: "secret",
		UseSSL:    false,
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := base.EnsureBucket(context.Background()); err != nil {
		t.Fatal(err)
	}
	wrapped := &streamingPutStore{Backend: base, started: make(chan struct{})}

	var lc net.ListenConfig
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcSrv := grpc.NewServer()
	server.Register(grpcSrv, wrapped)
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(grpcSrv.GracefulStop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := storagev1.NewStorageServiceClient(conn)
	ctx := context.Background()
	const chunk = 512 << 10
	payload := bytes.Repeat([]byte("z"), 2*chunk+1)

	put, err := client.Put(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := put.Send(&storagev1.PutRequest{Key: "large.bin", TotalSize: int64(len(payload)), Chunk: payload[:chunk]}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-wrapped.started:
	case <-time.After(5 * time.Second):
		t.Fatal("Put did not start streaming before all chunks were buffered")
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := put.Send(&storagev1.PutRequest{Chunk: payload[chunk:]}); err != nil {
			t.Errorf("second send: %v", err)
		}
		if _, err := put.CloseAndRecv(); err != nil {
			t.Errorf("CloseAndRecv: %v", err)
		}
	}()
	wg.Wait()
}

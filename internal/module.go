package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/Muxcore-Media/storage-ceph/internal/server"
	"github.com/Muxcore-Media/storage-ceph/internal/store"
)

// Module is a Ceph storage provider (RGW, CephFS mount, or native RADOS with -tags ceph).
// Laptop path: point CEPH_RGW_* at MinIO (see README / deploy/docker-compose.yml).
type Module struct { //nolint:govet // fieldalignment: lifecycle fields grouped for readability
	id          string
	grpcAddr    string
	httpAddr    string
	backend     string
	cephfsRoot  string
	monitors    string
	pool        string
	user        string
	keyring     string
	rgwEndpoint string
	bucket      string
	accessKey   string
	secretKey   string
	prefix      string
	cfgMu       sync.RWMutex
	store       store.Backend
	srv         *server.Server
	grpcSrv     *grpc.Server
	lis         net.Listener
	httpSrv     *http.Server
	useSSL      bool
	pathStyle   bool
}

type Config struct { //nolint:govet // fieldalignment: config fields grouped for readability
	ID          string
	Backend     string
	CephFSRoot  string
	Monitors    string
	Pool        string
	User        string
	Keyring     string
	RGWEndpoint string
	Bucket      string
	AccessKey   string
	SecretKey   string
	Prefix      string
	UseSSL      bool
	PathStyle   bool
	GRPCAddr    string
	HTTPAddr    string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "storage-ceph"
	}
	if cfg.Pool == "" {
		cfg.Pool = "muxcore"
	}
	if cfg.User == "" {
		cfg.User = "client.muxcore"
	}
	if cfg.RGWEndpoint == "" {
		cfg.RGWEndpoint = "127.0.0.1:7480"
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "muxcore"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9680"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9681"
	}
	cfg.PathStyle = true
	if v := os.Getenv("CEPH_STORAGE_BACKEND"); v != "" {
		cfg.Backend = v
	}
	if v := os.Getenv("CEPH_CEPHFS_ROOT"); v != "" {
		cfg.CephFSRoot = v
	}
	if v := os.Getenv("CEPH_MONITORS"); v != "" {
		cfg.Monitors = v
	}
	if v := os.Getenv("CEPH_POOL"); v != "" {
		cfg.Pool = v
	}
	if v := os.Getenv("CEPH_USER"); v != "" {
		cfg.User = v
	}
	if v := os.Getenv("CEPH_KEYRING"); v != "" {
		cfg.Keyring = v
	}
	if v := os.Getenv("CEPH_RGW_ENDPOINT"); v != "" {
		cfg.RGWEndpoint = v
	}
	if v := os.Getenv("CEPH_BUCKET"); v != "" {
		cfg.Bucket = v
	}
	if v := os.Getenv("CEPH_ACCESS_KEY"); v != "" {
		cfg.AccessKey = v
	}
	if v := os.Getenv("CEPH_SECRET_KEY"); v != "" {
		cfg.SecretKey = v
	}
	if v := os.Getenv("CEPH_PREFIX"); v != "" {
		cfg.Prefix = v
	}
	if v := os.Getenv("CEPH_USE_SSL"); v != "" {
		cfg.UseSSL = v == "1" || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("CEPH_PATH_STYLE"); v != "" {
		cfg.PathStyle = v == "1" || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("STORAGE_CEPH_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("STORAGE_CEPH_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	return &Module{
		id:          cfg.ID,
		backend:     cfg.Backend,
		cephfsRoot:  cfg.CephFSRoot,
		monitors:    cfg.Monitors,
		pool:        cfg.Pool,
		user:        cfg.User,
		keyring:     cfg.Keyring,
		rgwEndpoint: cfg.RGWEndpoint,
		bucket:      cfg.Bucket,
		accessKey:   cfg.AccessKey,
		secretKey:   cfg.SecretKey,
		prefix:      cfg.Prefix,
		useSSL:      cfg.UseSSL,
		pathStyle:   cfg.PathStyle,
		grpcAddr:    cfg.GRPCAddr,
		httpAddr:    cfg.HTTPAddr,
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Storage Ceph",
		Version:      "0.1.0",
		Roles:        []string{"storage", "infrastructure"},
		Description:  "Ceph/Rook RGW-backed StorageProvider (Put/Get/Delete/List/Stream)",
		Author:       "MuxCore",
		Capabilities: []string{"storage", "storage.ceph", "settings"},
		Contracts: []contracts.ContractDeclaration{
			{Repo: "github.com/Muxcore-Media/core/pkg/contracts", Interface: "StorageProvider", Version: "v0.5.4"},
		},
		MinCoreVersion: "0.5.4",
		HTTPAddr:       m.grpcAddr,
	}
}

func (m *Module) buildStore() (store.Backend, error) {
	return store.Open(store.ModuleConfig{
		Backend:    m.backend,
		Monitors:   m.monitors,
		Pool:       m.pool,
		User:       m.user,
		Keyring:    m.keyring,
		CephFSRoot: m.cephfsRoot,
		Prefix:     m.prefix,
		RGW: store.Config{
			Endpoint:  m.rgwEndpoint,
			Bucket:    m.bucket,
			AccessKey: m.accessKey,
			SecretKey: m.secretKey,
			Prefix:    m.prefix,
			UseSSL:    m.useSSL,
			PathStyle: m.pathStyle,
		},
	})
}

func (m *Module) Init(ctx context.Context) error {
	st, err := m.buildStore()
	if err != nil {
		return err
	}
	if ensureErr := st.EnsureBucket(ctx); ensureErr != nil {
		slog.Warn("storage-ceph: ensure bucket failed (will retry on use)", "error", ensureErr)
	}
	m.store = st

	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.grpcAddr, err)
	}
	m.lis = lis

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := m.Health(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	m.httpSrv = &http.Server{
		Addr:              m.httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("storage-ceph initialized",
		"backend", m.backendOrDefault(), "rgw", m.rgwEndpoint, "bucket", m.bucket, "pool", m.pool,
		"cephfs", m.cephfsRoot, "grpc", m.grpcAddr, "http", m.httpAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	m.srv = server.Register(m.grpcSrv, m.store)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)

	go func() {
		slog.Info("storage-ceph gRPC started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("storage-ceph gRPC serve error", "error", err)
		}
	}()
	go func() {
		slog.Info("storage-ceph HTTP started", "addr", m.httpAddr)
		if err := m.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("storage-ceph HTTP serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	slog.Info("storage-ceph stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	if m.store == nil {
		return fmt.Errorf("not initialized")
	}
	return m.store.Health(ctx)
}

func (m *Module) backendOrDefault() string {
	if strings.TrimSpace(m.backend) == "" {
		return "rgw"
	}
	return m.backend
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		_, err := strconv.ParseBool(v)
		return false, fmt.Errorf("invalid bool %q: %w", v, err)
	}
}

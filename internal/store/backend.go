package store

import (
	"context"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

// Backend is the storage implementation used by the gRPC server (RGW, CephFS, or RADOS).
type Backend interface {
	contracts.StorageProvider
	contracts.Streamable
	Health(ctx context.Context) error
	EnsureBucket(ctx context.Context) error
}

// Store is the default RGW (S3-compatible) backend.
type Store = RGWStore

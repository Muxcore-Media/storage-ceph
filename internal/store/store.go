package store

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

// Config holds Ceph RGW (S3-compatible) connection settings.
type Config struct {
	Endpoint   string
	Bucket     string
	Region     string
	AccessKey  string
	SecretKey  string
	Prefix     string
	UseSSL     bool
	PathStyle  bool
	CAFile     string
	ClientCert string
	ClientKey  string
}

// RGWStore implements contracts.StorageProvider (+ Streamable) against Ceph RGW.
type RGWStore struct {
	client *minio.Client
	bucket string
	prefix string
}

func New(cfg Config) (*RGWStore, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	endpoint := cfg.Endpoint
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimSuffix(endpoint, "/")

	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	if cfg.PathStyle {
		opts.BucketLookup = minio.BucketLookupPath
	}

	if cfg.UseSSL && cfg.CAFile != "" {
		transport, err := tlsTransport(cfg)
		if err != nil {
			return nil, err
		}
		opts.Transport = transport
	}

	client, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &RGWStore{
		client: client,
		bucket: cfg.Bucket,
		prefix: strings.Trim(cfg.Prefix, "/"),
	}, nil
}

func tlsTransport(cfg Config) (*http.Transport, error) {
	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("rgw ca %q: %w", cfg.CAFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("rgw ca %q: no certificates", cfg.CAFile)
	}
	tlsCfg := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	if cfg.ClientCert != "" && cfg.ClientKey != "" {
		cert, certErr := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if certErr != nil {
			return nil, fmt.Errorf("rgw client cert: %w", certErr)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsCfg
	return transport, nil
}

func (s *RGWStore) Close() error { return nil }

func (s *RGWStore) key(k string) string {
	k = strings.TrimPrefix(k, "/")
	if s.prefix == "" {
		return k
	}
	return s.prefix + "/" + k
}

func (s *RGWStore) Health(ctx context.Context) error {
	_, err := s.client.BucketExists(ctx, s.bucket)
	return err
}

func (s *RGWStore) EnsureBucket(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}

func (s *RGWStore) Put(ctx context.Context, key string, data io.Reader, size int64) error {
	opts := minio.PutObjectOptions{ContentType: "application/octet-stream"}
	_, err := s.client.PutObject(ctx, s.bucket, s.key(key), data, size, opts)
	return err
}

func (s *RGWStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.key(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, mapErr(err)
	}
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, mapErr(err)
	}
	return obj, nil
}

func (s *RGWStore) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, s.key(key), minio.RemoveObjectOptions{})
}

func (s *RGWStore) Move(ctx context.Context, src, dst string) error {
	srcKey, dstKey := s.key(src), s.key(dst)
	_, err := s.client.CopyObject(ctx,
		minio.CopyDestOptions{Bucket: s.bucket, Object: dstKey},
		minio.CopySrcOptions{Bucket: s.bucket, Object: srcKey},
	)
	if err != nil {
		return mapErr(err)
	}
	return s.client.RemoveObject(ctx, s.bucket, srcKey, minio.RemoveObjectOptions{})
}

func (s *RGWStore) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, s.key(key), minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *RGWStore) Stat(ctx context.Context, key string) (contracts.ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, s.key(key), minio.StatObjectOptions{})
	if err != nil {
		return contracts.ObjectInfo{}, mapErr(err)
	}
	return contracts.ObjectInfo{
		Key:          key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified,
		Metadata:     info.UserMetadata,
	}, nil
}

func (s *RGWStore) List(ctx context.Context, prefix string) ([]contracts.ObjectInfo, error) {
	full := s.key(prefix)
	if prefix == "" && s.prefix != "" {
		full = s.prefix + "/"
	}
	var out []contracts.ObjectInfo
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: full, Recursive: true}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		k := obj.Key
		if s.prefix != "" {
			k = strings.TrimPrefix(k, s.prefix+"/")
		}
		out = append(out, contracts.ObjectInfo{
			Key:          k,
			Size:         obj.Size,
			ETag:         obj.ETag,
			LastModified: obj.LastModified,
		})
	}
	return out, nil
}

// Stream implements contracts.Streamable via S3 range GETs.
func (s *RGWStore) Stream(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	opts := minio.GetObjectOptions{}
	switch {
	case length > 0:
		if err := opts.SetRange(offset, offset+length-1); err != nil {
			return nil, err
		}
	case offset > 0:
		// Open-ended range from offset to EOF.
		if err := opts.SetRange(offset, 0); err != nil {
			return nil, err
		}
	}
	obj, err := s.client.GetObject(ctx, s.bucket, s.key(key), opts)
	if err != nil {
		return nil, mapErr(err)
	}
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, mapErr(err)
	}
	return obj, nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if isNotFound(err) {
		return contracts.ErrNotFound
	}
	return err
}

func isNotFound(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || resp.Code == "NotFound" || resp.StatusCode == 404
}

var (
	_ Backend                   = (*RGWStore)(nil)
	_ contracts.StorageProvider = (*RGWStore)(nil)
	_ contracts.Streamable      = (*RGWStore)(nil)
)

package internal

import (
	"fmt"
	"os"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/Muxcore-Media/storage-ceph/internal/store"
)

func (m *Module) Settings() []contracts.SettingDef {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return []contracts.SettingDef{
		{
			Key: "storage_backend", Label: "Storage Backend", Type: contracts.SettingTypeSelect,
			Value: m.backendOrDefault(), Default: "rgw",
			Options:     []string{"rgw", "cephfs", "rados"},
			Description: "Active backend: rgw (S3/RGW), cephfs (mounted volume), or rados (native librados); CEPH_STORAGE_BACKEND",
			Group:       "Ceph",
		},
		{Key: "cephfs_root", Label: "CephFS Mount Root", Type: contracts.SettingTypeString, Value: m.cephfsRoot, Description: "Mounted CephFS path when backend=cephfs; CEPH_CEPHFS_ROOT", Group: "Ceph"},
		{Key: "monitors", Label: "Ceph Monitors", Type: contracts.SettingTypeString, Value: m.monitors, Description: "Comma-separated mon endpoints (Rook/docs); CEPH_MONITORS", Group: "Ceph"},
		{Key: "pool", Label: "RADOS Pool", Type: contracts.SettingTypeString, Value: m.pool, Default: "muxcore", Description: "RADOS pool when backend=rados; CEPH_POOL", Group: "Ceph"},
		{Key: "user", Label: "Ceph User", Type: contracts.SettingTypeString, Value: m.user, Default: "client.muxcore", Description: "CEPH_USER", Group: "Ceph"},
		{Key: "keyring", Label: "Keyring Path", Type: contracts.SettingTypeSecret, Value: modulesdk.MaskSecret(m.keyring), Description: "Filesystem path to ceph keyring when backend=rados; CEPH_KEYRING", Group: "Ceph"},
		{Key: "rgw_endpoint", Label: "RGW Endpoint", Type: contracts.SettingTypeString, Value: m.rgwEndpoint, Default: "127.0.0.1:7480", Description: "Ceph Object Gateway host:port; CEPH_RGW_ENDPOINT", Group: "RGW"},
		{Key: "bucket", Label: "Bucket", Type: contracts.SettingTypeString, Value: m.bucket, Default: "muxcore", Description: "RGW bucket; CEPH_BUCKET", Group: "RGW"},
		{Key: "access_key", Label: "Access Key", Type: contracts.SettingTypeSecret, Value: modulesdk.MaskSecret(m.accessKey), Description: "RGW S3 access key; CEPH_ACCESS_KEY", Group: "RGW"},
		{Key: "secret_key", Label: "Secret Key", Type: contracts.SettingTypeSecret, Value: modulesdk.MaskSecret(m.secretKey), Description: "RGW S3 secret key; CEPH_SECRET_KEY", Group: "RGW"},
		{Key: "prefix", Label: "Key Prefix", Type: contracts.SettingTypeString, Value: m.prefix, Description: "Optional object key prefix; CEPH_PREFIX", Group: "RGW"},
		{Key: "use_ssl", Label: "Use SSL", Type: contracts.SettingTypeBool, Value: fmt.Sprintf("%t", m.useSSL), Default: "false", Description: "CEPH_USE_SSL", Group: "RGW"},
		{Key: "rgw_ca", Label: "RGW CA Bundle", Type: contracts.SettingTypeString, Value: m.rgwCA, Description: "PEM file for Rook/private RGW CA when use_ssl=true; CEPH_RGW_CA", Group: "RGW"},
		{Key: "rgw_client_cert", Label: "RGW Client Cert", Type: contracts.SettingTypeString, Value: m.rgwClientCert, Description: "Optional mTLS client certificate path; CEPH_RGW_CLIENT_CERT", Group: "RGW"},
		{Key: "rgw_client_key", Label: "RGW Client Key", Type: contracts.SettingTypeSecret, Value: modulesdk.MaskSecret(m.rgwClientKey), Description: "Optional mTLS client key path; CEPH_RGW_CLIENT_KEY", Group: "RGW"},
		{Key: "path_style", Label: "Path-Style Addressing", Type: contracts.SettingTypeBool, Value: fmt.Sprintf("%t", m.pathStyle), Default: "true", Description: "CEPH_PATH_STYLE", Group: "RGW"},
	}
}

func (m *Module) UpdateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()

	snap := m.snapshotLocked()
	if err := m.applySettingLocked(key, value); err != nil {
		return err
	}
	if m.store == nil {
		return nil
	}
	st, err := m.buildStore()
	if err != nil {
		m.restoreLocked(snap)
		return err
	}
	var old store.Backend
	if m.srv != nil {
		old = m.srv.SwapStore(st)
	} else {
		old = m.store
	}
	m.store = st
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (m *Module) applySettingLocked(key, value string) error {
	switch key {
	case "storage_backend", "CEPH_STORAGE_BACKEND":
		return m.setStorageBackend(value)
	case "cephfs_root", "CEPH_CEPHFS_ROOT":
		return m.setCephFSRoot(value)
	case "monitors", "CEPH_MONITORS":
		m.monitors = value
	case "pool", "CEPH_POOL":
		return m.setPool(value)
	case "user", "CEPH_USER":
		m.user = value
	case "keyring", "CEPH_KEYRING":
		return m.setKeyringPath(value)
	case "rgw_endpoint", "CEPH_RGW_ENDPOINT":
		return m.setRGWEndpoint(value)
	case "bucket", "CEPH_BUCKET":
		return m.setBucket(value)
	case "access_key", "CEPH_ACCESS_KEY":
		return m.setSecret(&m.accessKey, value)
	case "secret_key", "CEPH_SECRET_KEY":
		return m.setSecret(&m.secretKey, value)
	case "prefix", "CEPH_PREFIX":
		m.prefix = value
	case "use_ssl", "CEPH_USE_SSL":
		return m.setUseSSL(value)
	case "rgw_ca", "CEPH_RGW_CA":
		m.rgwCA = value
	case "rgw_client_cert", "CEPH_RGW_CLIENT_CERT":
		m.rgwClientCert = value
	case "rgw_client_key", "CEPH_RGW_CLIENT_KEY":
		return m.setSecret(&m.rgwClientKey, value)
	case "path_style", "CEPH_PATH_STYLE":
		return m.setPathStyle(value)
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}

func (m *Module) setStorageBackend(value string) error {
	v := strings.ToLower(value)
	if v != "" && v != "rgw" && v != "s3" && v != "cephfs" && v != "fs" && v != "rados" {
		return fmt.Errorf("storage_backend must be rgw, cephfs, or rados")
	}
	if v == "s3" {
		v = "rgw"
	}
	if v == "fs" {
		v = "cephfs"
	}
	if v != "" {
		m.backend = v
	}
	return nil
}

func (m *Module) setCephFSRoot(value string) error {
	if value == "" {
		return fmt.Errorf("cephfs_root must not be empty when set")
	}
	m.cephfsRoot = value
	return nil
}

func (m *Module) setPool(value string) error {
	if value == "" {
		return fmt.Errorf("pool must not be empty")
	}
	m.pool = value
	return nil
}

func (m *Module) setKeyringPath(value string) error {
	if value == "********" {
		return nil
	}
	if value != "" {
		if err := validateKeyringValue(value, m.backendOrDefault() == "rados"); err != nil {
			return err
		}
	}
	m.keyring = value
	return nil
}

func validateKeyringValue(value string, requireFile bool) error {
	if strings.Contains(value, "\n") || strings.HasPrefix(strings.TrimSpace(value), "[") {
		return fmt.Errorf("keyring must be a filesystem path, not inline key material")
	}
	if !requireFile {
		return nil
	}
	info, err := os.Stat(value)
	if err != nil {
		return fmt.Errorf("keyring path %q: %w", value, err)
	}
	if info.IsDir() {
		return fmt.Errorf("keyring path %q is a directory", value)
	}
	return nil
}

func (m *Module) setSecret(dst *string, value string) error {
	if value == "********" {
		return nil
	}
	*dst = value
	return nil
}

func (m *Module) setRGWEndpoint(value string) error {
	if value == "" {
		return fmt.Errorf("rgw_endpoint must not be empty")
	}
	m.rgwEndpoint = value
	return nil
}

func (m *Module) setBucket(value string) error {
	if value == "" {
		return fmt.Errorf("bucket must not be empty")
	}
	m.bucket = value
	return nil
}

func (m *Module) setUseSSL(value string) error {
	b, err := parseBool(value)
	if err != nil {
		return err
	}
	m.useSSL = b
	return nil
}

func (m *Module) setPathStyle(value string) error {
	b, err := parseBool(value)
	if err != nil {
		return err
	}
	m.pathStyle = b
	return nil
}

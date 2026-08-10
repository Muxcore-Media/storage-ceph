package internal

import (
	"fmt"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/Muxcore-Media/storage-ceph/internal/store"
)

func (m *Module) Settings() []contracts.SettingDef {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return []contracts.SettingDef{
		{Key: "monitors", Label: "Ceph Monitors", Type: contracts.SettingTypeString, Value: m.monitors, Description: "Comma-separated mon endpoints (Rook/docs); CEPH_MONITORS", Group: "Ceph"},
		{Key: "pool", Label: "RADOS Pool", Type: contracts.SettingTypeString, Value: m.pool, Default: "muxcore", Description: "Target pool for future native RADOS; CEPH_POOL", Group: "Ceph"},
		{Key: "user", Label: "Ceph User", Type: contracts.SettingTypeString, Value: m.user, Default: "client.muxcore", Description: "CEPH_USER", Group: "Ceph"},
		{Key: "keyring", Label: "Keyring Path", Type: contracts.SettingTypeSecret, Value: modulesdk.MaskSecret(m.keyring), Description: "Path or key material; CEPH_KEYRING", Group: "Ceph"},
		{Key: "rgw_endpoint", Label: "RGW Endpoint", Type: contracts.SettingTypeString, Value: m.rgwEndpoint, Default: "127.0.0.1:7480", Description: "Ceph Object Gateway host:port; CEPH_RGW_ENDPOINT", Group: "RGW"},
		{Key: "bucket", Label: "Bucket", Type: contracts.SettingTypeString, Value: m.bucket, Default: "muxcore", Description: "RGW bucket; CEPH_BUCKET", Group: "RGW"},
		{Key: "access_key", Label: "Access Key", Type: contracts.SettingTypeSecret, Value: modulesdk.MaskSecret(m.accessKey), Description: "RGW S3 access key; CEPH_ACCESS_KEY", Group: "RGW"},
		{Key: "secret_key", Label: "Secret Key", Type: contracts.SettingTypeSecret, Value: modulesdk.MaskSecret(m.secretKey), Description: "RGW S3 secret key; CEPH_SECRET_KEY", Group: "RGW"},
		{Key: "prefix", Label: "Key Prefix", Type: contracts.SettingTypeString, Value: m.prefix, Description: "Optional object key prefix; CEPH_PREFIX", Group: "RGW"},
		{Key: "use_ssl", Label: "Use SSL", Type: contracts.SettingTypeBool, Value: fmt.Sprintf("%t", m.useSSL), Default: "false", Description: "CEPH_USE_SSL", Group: "RGW"},
		{Key: "path_style", Label: "Path-Style Addressing", Type: contracts.SettingTypeBool, Value: fmt.Sprintf("%t", m.pathStyle), Default: "true", Description: "CEPH_PATH_STYLE", Group: "RGW"},
	}
}

func (m *Module) UpdateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()

	switch key {
	case "monitors", "CEPH_MONITORS":
		m.monitors = value
		return nil
	case "pool", "CEPH_POOL":
		if value == "" {
			return fmt.Errorf("pool must not be empty")
		}
		m.pool = value
		return nil
	case "user", "CEPH_USER":
		m.user = value
		return nil
	case "keyring", "CEPH_KEYRING":
		if value == "********" {
			return nil
		}
		m.keyring = value
		return nil
	case "rgw_endpoint", "CEPH_RGW_ENDPOINT":
		if value == "" {
			return fmt.Errorf("rgw_endpoint must not be empty")
		}
		m.rgwEndpoint = value
	case "bucket", "CEPH_BUCKET":
		if value == "" {
			return fmt.Errorf("bucket must not be empty")
		}
		m.bucket = value
	case "access_key", "CEPH_ACCESS_KEY":
		if value == "********" {
			return nil
		}
		m.accessKey = value
	case "secret_key", "CEPH_SECRET_KEY":
		if value == "********" {
			return nil
		}
		m.secretKey = value
	case "prefix", "CEPH_PREFIX":
		m.prefix = value
	case "use_ssl", "CEPH_USE_SSL":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		m.useSSL = b
	case "path_style", "CEPH_PATH_STYLE":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		m.pathStyle = b
	default:
		return fmt.Errorf("unknown setting %q", key)
	}

	if m.store == nil {
		return nil
	}
	st, err := store.New(store.Config{
		Endpoint:  m.rgwEndpoint,
		Bucket:    m.bucket,
		AccessKey: m.accessKey,
		SecretKey: m.secretKey,
		Prefix:    m.prefix,
		UseSSL:    m.useSSL,
		PathStyle: m.pathStyle,
	})
	if err != nil {
		return err
	}
	m.store = st
	if m.srv != nil {
		m.srv.ReplaceStore(st)
	}
	return nil
}

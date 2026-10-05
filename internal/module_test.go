package internal

import (
	"testing"

	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	manifest "github.com/Muxcore-Media/storage-ceph"
)

func TestModuleInfo_StorageCapability(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	info := m.Info()
	if info.ID != "storage-ceph" {
		t.Fatalf("ID=%q", info.ID)
	}
	if info.Version != modulesdk.ManifestVersion(manifest.ManifestJSON) {
		t.Fatalf("Version=%q", info.Version)
	}
	want := map[string]bool{"storage": false, "storage.ceph": false, "settings": false}
	for _, c := range info.Capabilities {
		if _, ok := want[c]; ok {
			want[c] = true
		}
	}
	for c, ok := range want {
		if !ok {
			t.Fatalf("missing capability %q in %v", c, info.Capabilities)
		}
	}
}

func TestSettings_MaskSecrets(t *testing.T) {
	m := NewModule(Config{
		AccessKey:   "ak",
		SecretKey:   "sk",
		Keyring:     "/etc/ceph/keyring",
		RGWEndpoint: "127.0.0.1:9000",
		Bucket:      "muxcore",
	})
	defs := m.Settings()
	byKey := map[string]string{}
	for _, d := range defs {
		byKey[d.Key] = d.Value
	}
	if byKey["access_key"] != modulesdk.MaskSecret("ak") {
		t.Fatalf("access_key=%q", byKey["access_key"])
	}
	if byKey["secret_key"] != modulesdk.MaskSecret("sk") {
		t.Fatalf("secret_key=%q", byKey["secret_key"])
	}
	if byKey["keyring"] != modulesdk.MaskSecret("/etc/ceph/keyring") {
		t.Fatalf("keyring=%q", byKey["keyring"])
	}
	if byKey["storage_backend"] != "rgw" {
		t.Fatalf("storage_backend=%q", byKey["storage_backend"])
	}
}

func TestUpdateSetting_Validation(t *testing.T) {
	m := NewModule(Config{Bucket: "muxcore", RGWEndpoint: "127.0.0.1:9000"})
	if err := m.UpdateSetting("storage_backend", "nfs"); err == nil {
		t.Fatal("expected reject unknown backend")
	}
	if err := m.UpdateSetting("bucket", ""); err == nil {
		t.Fatal("expected reject empty bucket")
	}
	if err := m.UpdateSetting("pool", ""); err == nil {
		t.Fatal("expected reject empty pool")
	}
	if err := m.UpdateSetting("keyring", "[client.admin]\nkey = abc"); err == nil {
		t.Fatal("expected reject inline keyring")
	}
}

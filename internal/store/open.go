package store

import (
	"fmt"
	"strings"
)

// RADOSConfig holds native librados connection settings.
type RADOSConfig struct {
	Monitors string
	Pool     string
	User     string
	Keyring  string
	Prefix   string
}

// ModuleConfig selects the active storage backend.
type ModuleConfig struct {
	Backend    string // rgw (default), cephfs, rados
	Monitors   string
	Pool       string
	User       string
	Keyring    string
	CephFSRoot string
	Prefix     string
	RGW        Config
}

func Open(cfg ModuleConfig) (Backend, error) {
	backend := strings.ToLower(strings.TrimSpace(cfg.Backend))
	if backend == "" {
		backend = "rgw"
	}
	switch backend {
	case "rgw", "s3":
		st, err := New(cfg.RGW)
		if err != nil {
			return nil, err
		}
		return st, nil
	case "cephfs", "fs":
		return NewCephFS(CephFSConfig{Root: cfg.CephFSRoot, Prefix: cfg.Prefix})
	case "rados":
		return NewRADOS(RADOSConfig{
			Monitors: cfg.Monitors,
			Pool:     cfg.Pool,
			User:     cfg.User,
			Keyring:  cfg.Keyring,
			Prefix:   cfg.Prefix,
		})
	default:
		return nil, fmt.Errorf("unknown storage backend %q (want rgw, cephfs, or rados)", cfg.Backend)
	}
}

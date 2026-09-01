//go:build ceph

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/ceph/go-ceph/rados"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

type cephRadosIO struct {
	conn  *rados.Conn
	ioctx *rados.IOContext
}

func (c *cephRadosIO) Write(name string, data []byte, offset uint64) error {
	return c.ioctx.Write(name, data, offset)
}

func (c *cephRadosIO) Read(name string, offset, length uint64) ([]byte, error) {
	return c.ioctx.Read(name, offset, length)
}

func (c *cephRadosIO) Delete(name string) error {
	err := c.ioctx.Delete(name)
	if err != nil && rados.ErrNotFound == err {
		return nil
	}
	return err
}

func (c *cephRadosIO) Stat(name string) (radosObjectStat, error) {
	stat, err := c.ioctx.Stat(name)
	if err != nil {
		if rados.ErrNotFound == err {
			return radosObjectStat{}, contracts.ErrNotFound
		}
		return radosObjectStat{}, err
	}
	return radosObjectStat{Size: stat.Size}, nil
}

func (c *cephRadosIO) Iter() (radosIterator, error) {
	iter, err := c.ioctx.Iter()
	if err != nil {
		return nil, err
	}
	return &cephRadosIter{iter: iter}, nil
}

func (c *cephRadosIO) Ping(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_, err := c.conn.GetFSID()
	return err
}

func (c *cephRadosIO) Close() error {
	if c.ioctx != nil {
		c.ioctx.Destroy()
		c.ioctx = nil
	}
	if c.conn != nil {
		c.conn.Shutdown()
		c.conn = nil
	}
	return nil
}

type cephRadosIter struct {
	iter *rados.Iter
}

func (i *cephRadosIter) Next() bool { return i.iter.Next() }

func (i *cephRadosIter) Value() string { return i.iter.Value().Name }

func (i *cephRadosIter) Err() error { return i.iter.Err() }

func (i *cephRadosIter) Close() { i.iter.Close() }

// NewRADOS connects to a Ceph cluster via librados (build tag ceph).
func NewRADOS(cfg RADOSConfig) (Backend, error) {
	if strings.TrimSpace(cfg.Pool) == "" {
		return nil, fmt.Errorf("rados pool is required")
	}
	conn, err := rados.NewConnWithUser(cfg.User)
	if err != nil {
		return nil, fmt.Errorf("rados conn: %w", err)
	}
	for _, mon := range strings.Split(cfg.Monitors, ",") {
		mon = strings.TrimSpace(mon)
		if mon == "" {
			continue
		}
		if err := conn.AddMonitor(mon); err != nil {
			conn.Shutdown()
			return nil, fmt.Errorf("add monitor %q: %w", mon, err)
		}
	}
	if cfg.Keyring != "" {
		if err := conn.ReadConfigFile(cfg.Keyring); err != nil {
			if err := conn.SetConfigOption("key", cfg.Keyring); err != nil {
				conn.Shutdown()
				return nil, fmt.Errorf("keyring %q: %w", cfg.Keyring, err)
			}
		}
	}
	if err := conn.Connect(); err != nil {
		conn.Shutdown()
		return nil, fmt.Errorf("rados connect: %w", err)
	}
	ioctx, err := conn.OpenIOContext(cfg.Pool)
	if err != nil {
		conn.Shutdown()
		return nil, fmt.Errorf("open pool %q: %w", cfg.Pool, err)
	}
	return newRADOSStore(&cephRadosIO{conn: conn, ioctx: ioctx}, cfg.Prefix), nil
}

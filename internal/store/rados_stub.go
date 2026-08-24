//go:build !ceph

package store

import "fmt"

// NewRADOS is implemented in rados_ceph.go when built with -tags ceph.
func NewRADOS(cfg RADOSConfig) (Backend, error) {
	return nil, fmt.Errorf("RADOS backend requires storage-ceph built with -tags ceph and librados installed")
}

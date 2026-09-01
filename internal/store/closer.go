package store

import "io"

// limitReadCloser wraps a limited reader and closes the underlying ReadCloser.
type limitReadCloser struct {
	io.Reader
	closer io.Closer
}

func (l *limitReadCloser) Close() error {
	return l.closer.Close()
}

func limitRC(r io.ReadCloser, n int64) io.ReadCloser {
	if n > 0 {
		return &limitReadCloser{Reader: io.LimitReader(r, n), closer: r}
	}
	return r
}

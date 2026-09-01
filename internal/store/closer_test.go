package store

import (
	"io"
	"strings"
	"testing"
)

type trackedRC struct {
	io.Reader
	closed bool
}

func (t *trackedRC) Close() error {
	t.closed = true
	return nil
}

func TestLimitRCClosesInner(t *testing.T) {
	inner := &trackedRC{Reader: strings.NewReader("abcd")}
	rc := limitRC(inner, 4)
	if _, err := io.ReadAll(rc); err != nil {
		t.Fatal(err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	if !inner.closed {
		t.Fatal("inner ReadCloser was not closed")
	}
}

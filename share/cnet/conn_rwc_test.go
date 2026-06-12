package cnet

import (
	"io"
	"testing"
)

type fakeRWC struct {
	io.ReadWriteCloser
	closedWrite bool
}

func (f *fakeRWC) Read([]byte) (int, error)  { return 0, io.EOF }
func (f *fakeRWC) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (f *fakeRWC) Close() error              { return nil }
func (f *fakeRWC) CloseWrite() error {
	f.closedWrite = true
	return nil
}

// TestRWCConnCloseWrite verifies half-closes are delegated to the
// underlying stream when supported (e.g. ssh.Channel for socks conns).
func TestRWCConnCloseWrite(t *testing.T) {
	f := &fakeRWC{}
	c := NewRWCConn(f)
	cw, ok := c.(interface{ CloseWrite() error })
	if !ok {
		t.Fatal("rwcConn does not expose CloseWrite")
	}
	if err := cw.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if !f.closedWrite {
		t.Fatal("CloseWrite was not delegated to the underlying stream")
	}
}

package chshare

import (
	"os"
)


type stdio struct {}

var Stdio = &stdio{}

func (t *stdio) Read(b []byte) (n int, err error) {
	return os.Stdin.Read(b)
}

func (t *stdio) Write(b []byte) (n int, err error) {
	return os.Stdout.Write(b)
}

// We do not close stdio
func (t *stdio) Close() error {
	return nil
}

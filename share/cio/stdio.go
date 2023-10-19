package cio

import (
	"io"
	"os"
)

//Stdio as a ReadWriteCloser
var Stdio = &struct {
	io.ReadCloser
	io.Writer
}{
	io.NopCloser(os.Stdin),
	os.Stdout,
}

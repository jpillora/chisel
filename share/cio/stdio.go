package cio

import (
	"io"
	"io/ioutil"
	"os"
)

//Stdio as a ReadWriteCloser
var Stdio = &struct {
	io.ReadCloser
	io.Writer
}{
	ioutil.NopCloser(os.Stdin),
	os.Stdout,
}

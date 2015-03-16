package chshare

import (
	"errors"
	"net"
	"net/http"
	"sync"
)

//HTTPServer extends net/http Server and
//adds graceful shutdowns
type HTTPServer struct {
	*http.Server
	listener  net.Listener
	running   chan error
	isRunning bool
	closer    sync.Once
}

//NewHTTPServer creates a new HTTPServer
func NewHTTPServer() *HTTPServer {
	return &HTTPServer{
		Server:   &http.Server{},
		listener: nil,
		running:  make(chan error, 1),
	}
}

func (h *HTTPServer) GoListenAndServe(addr string, handler http.Handler) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	h.isRunning = true
	h.Handler = handler
	h.listener = l
	go func() {
		h.closeWith(h.Serve(l))
	}()
	return nil
}

func (h *HTTPServer) closeWith(err error) {
	if !h.isRunning {
		return
	}
	h.isRunning = false
	h.running <- err
}

func (h *HTTPServer) Close() error {
	h.closeWith(nil)
	return h.listener.Close()
}

func (h *HTTPServer) Wait() error {
	if !h.isRunning {
		return errors.New("Already closed")
	}
	return <-h.running
}

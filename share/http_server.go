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
	return h.listenAndServe(addr, handler, h.Serve)
}

func (h *HTTPServer) GoListenAndServeTLS(addr string, handler http.Handler, certFile, privateKeyFile string) error {
	return h.listenAndServe(addr, handler, func(l net.Listener) error {
		return h.ServeTLS(l, certFile, privateKeyFile)
	})
}

func (h *HTTPServer) listenAndServe(addr string, handler http.Handler, serveFunc func(l net.Listener) error) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	h.isRunning = true
	h.Handler = handler
	h.listener = l

	go func() {
		h.closeWith(serveFunc(l))
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

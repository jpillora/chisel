package cnet

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
)

//HTTPServer extends net/http Server and
//adds graceful shutdowns
type HTTPServer struct {
	*http.Server
	serving   bool
	waiter    sync.WaitGroup
	listenErr error
}

//NewHTTPServer creates a new HTTPServer
func NewHTTPServer() *HTTPServer {
	return &HTTPServer{
		Server:  &http.Server{},
		serving: false,
	}

}

func (h *HTTPServer) GoListenAndServe(addr string, handler http.Handler) error {
	return h.GoListenAndServeContext(nil, addr, handler)
}

func (h *HTTPServer) GoListenAndServeContext(ctx context.Context, addr string, handler http.Handler) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	h.Handler = handler
	h.serving = true
	h.waiter.Add(1)
	go func() {
		h.listenErr = h.Serve(l)
		h.waiter.Done()
	}()
	if ctx != nil {
		go func() {
			<-ctx.Done()
			h.Close()
		}()
	}
	return nil
}

func (h *HTTPServer) Close() error {
	if !h.serving {
		return errors.New("not started yet")
	}
	err := h.Server.Close()
	h.waiter.Wait()
	return err
}

func (h *HTTPServer) Wait() error {
	if !h.serving {
		return errors.New("not started yet")
	}
	h.waiter.Wait()
	return h.listenErr
}

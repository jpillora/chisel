package cnet

import (
	"context"
	"errors"
	"net"
	"net/http"

	"golang.org/x/sync/errgroup"
)

//HTTPServer extends net/http Server and
//adds graceful shutdowns
type HTTPServer struct {
	*http.Server
	serving   bool
	waiter    *errgroup.Group
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
	return h.GoListenAndServeContext(context.Background(), addr, handler)
}

func (h *HTTPServer) GoListenAndServeContext(ctx context.Context, addr string, handler http.Handler) error {
	if ctx == nil {
		return errors.New("ctx must be set")
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return h.GoServe(ctx, l, handler)
}

func (h *HTTPServer) GoServe(ctx context.Context, l net.Listener, handler http.Handler) error {
	if ctx == nil {
		return errors.New("ctx must be set")
	}
	h.Handler = handler
	h.serving = true
	h.waiter, ctx = errgroup.WithContext(ctx)
	h.waiter.Go(func() error {
		return h.Serve(l)
	})
	go func() {
		<-ctx.Done()
		h.Close()
	}()
	return nil
}

func (h *HTTPServer) Close() error {
	if !h.serving {
		return errors.New("not started yet")
	}
	return h.Server.Close()
}

func (h *HTTPServer) Wait() error {
	if !h.serving {
		return errors.New("not started yet")
	}
	err := h.waiter.Wait()
	if err == http.ErrServerClosed {
		err = nil //success
	}
	return err
}

package cnet

import (
	"context"
	"errors"
	"github.com/lucas-clemente/quic-go/http3"
	"net"
	"net/http"
	"sync"

	"golang.org/x/sync/errgroup"
)

//HTTP3Server extends net/http Server and
//adds graceful shutdowns
type HTTP3Server struct {
	*http3.Server
	waiterMux sync.Mutex
	waiter    *errgroup.Group
	listenErr error
}

//NewHTTP3Server creates a new HTTPServer
func NewHTTP3Server() *HTTP3Server {
	return &HTTP3Server{
		Server: &http3.Server{},
	}

}

func (h *HTTP3Server) GoListenAndServe(addr string, handler http.Handler) error {
	return h.GoListenAndServeContext(context.Background(), addr, handler)
}

func (h *HTTP3Server) GoListenAndServeContext(ctx context.Context, addr string, handler http.Handler) error {
	if ctx == nil {
		return errors.New("ctx must be set")
	}

	l, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	return h.GoServe(ctx, l, handler)
}

func (h *HTTP3Server) GoServe(ctx context.Context, conn net.PacketConn, handler http.Handler) error {
	if ctx == nil {
		return errors.New("ctx must be set")
	}
	h.waiterMux.Lock()
	defer h.waiterMux.Unlock()
	h.Handler = handler
	h.waiter, ctx = errgroup.WithContext(ctx)
	h.waiter.Go(func() error {
		return h.Serve(conn)
	})
	go func() {
		<-ctx.Done()
		h.Close()
	}()
	return nil
}

func (h *HTTP3Server) Close() error {
	h.waiterMux.Lock()
	defer h.waiterMux.Unlock()
	if h.waiter == nil {
		return errors.New("not started yet")
	}
	return h.Server.Close()
}

func (h *HTTP3Server) Wait() error {
	h.waiterMux.Lock()
	unset := h.waiter == nil
	h.waiterMux.Unlock()
	if unset {
		return errors.New("not started yet")
	}
	h.waiterMux.Lock()
	wait := h.waiter.Wait
	h.waiterMux.Unlock()
	err := wait()
	if err == http.ErrServerClosed {
		err = nil //success
	}
	return err
}

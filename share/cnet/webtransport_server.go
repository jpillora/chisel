package cnet

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"sync"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
	"golang.org/x/sync/errgroup"
)

// WebTransportServer extends webtransport Server and
// adds graceful shutdowns
type WebTransportServer struct {
	*webtransport.Server
	waiterMux sync.Mutex
	waiter    *errgroup.Group
	listenErr error
}

// NewWebTransportServer creates a new WebTransportServer
func NewWebTransportServer() *WebTransportServer {
	return &WebTransportServer{
		Server: &webtransport.Server{},
	}

}

func (h *WebTransportServer) GoListenAndServe(addr string, tlsConf *tls.Config, config *quic.Config, handler http.Handler) error {
	return h.GoListenAndServeContext(context.Background(), addr, tlsConf, config, handler)
}

func (h *WebTransportServer) GoListenAndServeContext(ctx context.Context, addr string, tlsConf *tls.Config, config *quic.Config, handler http.Handler) error {
	if ctx == nil {
		return errors.New("ctx must be set")
	}
	l, err := quic.ListenAddrEarly(addr, tlsConf, config)
	if err != nil {
		return err
	}
	return h.GoServe(ctx, l, handler)
}

func (h *WebTransportServer) GoServe(ctx context.Context, l quic.EarlyListener, handler http.Handler) error {
	if ctx == nil {
		return errors.New("ctx must be set")
	}
	h.waiterMux.Lock()
	defer h.waiterMux.Unlock()
	h.H3.Handler = handler
	h.waiter, ctx = errgroup.WithContext(ctx)
	h.waiter.Go(func() error {
		return h.ServeListener(l)
	})
	go func() {
		<-ctx.Done()
		h.Close()
	}()
	return nil
}

func (h *WebTransportServer) Close() error {
	h.waiterMux.Lock()
	defer h.waiterMux.Unlock()
	if h.waiter == nil {
		return errors.New("not started yet")
	}
	return h.Server.Close()
}

func (h *WebTransportServer) Wait() error {
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

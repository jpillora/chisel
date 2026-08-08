package cnet

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/jpillora/chisel/share/settings"
	"golang.org/x/sync/errgroup"
)

// HTTPServer extends net/http Server and
// adds graceful shutdowns
type HTTPServer struct {
	*http.Server
	waiterMux sync.Mutex
	waiter    *errgroup.Group
}

// NewHTTPServer creates a new HTTPServer
func NewHTTPServer() *HTTPServer {
	return &HTTPServer{
		Server: &http.Server{},
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
	h.waiterMux.Lock()
	defer h.waiterMux.Unlock()
	h.Handler = handler
	h.waiter, ctx = errgroup.WithContext(ctx)
	h.waiter.Go(func() error {
		return h.Serve(l)
	})
	//run the drain inside the group so Wait observes it. ctx here is the
	//group's derived context, so this also unblocks when Serve returns —
	//that is what lets a bare Close() (no cancellation) still finish Wait.
	//Serve never returns nil: it reports ErrServerClosed on Close/Shutdown.
	h.waiter.Go(func() error {
		<-ctx.Done()
		//graceful shutdown: stop accepting, drain in-flight requests
		//for a grace period, then force-close the remainder. hijacked
		//connections (websocket tunnels) are closed by their owners.
		grace := settings.EnvDuration("SHUTDOWN_GRACE", 5*time.Second)
		c, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		h.Server.Shutdown(c)
		h.Close()
		return nil
	})
	return nil
}

func (h *HTTPServer) Close() error {
	h.waiterMux.Lock()
	defer h.waiterMux.Unlock()
	if h.waiter == nil {
		return errors.New("not started yet")
	}
	return h.Server.Close()
}

func (h *HTTPServer) Wait() error {
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

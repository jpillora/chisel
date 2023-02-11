package chserver

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	activeSessions = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "chisel_number_of_active_sessions",
		Help: "The number of active sessions on this chisel server.",
	})
)

func (s *Server) exposeMetrics() {
	register()
	activeSessions.Set(0)

	// Run second HTTP server in another goroutine
	go func() {
		// The Handler function provides a default handler to expose metrics
		// via an HTTP server. "/metrics" is the usual endpoint for that.
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":9113", nil)
	}()
}

func register() {
	prometheus.MustRegister(activeSessions)
}
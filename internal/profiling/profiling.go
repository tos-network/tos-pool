// Package profiling provides pprof profiling server for debugging.
package profiling

import (
	"net/http"
	"net/http/pprof"

	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/util"
)

// Server provides pprof profiling endpoints
type Server struct {
	cfg    *config.ProfilingConfig
	server *http.Server
}

// NewServer creates a new profiling server
func NewServer(cfg *config.ProfilingConfig) *Server {
	return &Server{
		cfg: cfg,
	}
}

// Start begins the profiling server
func (s *Server) Start() error {
	if !s.cfg.Enabled {
		return nil
	}

	mux := http.NewServeMux()

	// Register pprof handlers
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))

	s.server = &http.Server{
		Addr:    s.cfg.Bind,
		Handler: mux,
	}

	util.Infof("pprof profiling server listening on %s", s.cfg.Bind)
	util.Info("  Available endpoints:")
	util.Info("    /debug/pprof/         - Index")
	util.Info("    /debug/pprof/goroutine - Goroutine stack traces")
	util.Info("    /debug/pprof/heap     - Heap profile")
	util.Info("    /debug/pprof/profile  - CPU profile (30s)")
	util.Info("    /debug/pprof/trace    - Execution trace")

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			util.Errorf("Profiling server error: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the profiling server
func (s *Server) Stop() error {
	if s.server != nil {
		util.Info("Stopping profiling server")
		return s.server.Close()
	}
	return nil
}

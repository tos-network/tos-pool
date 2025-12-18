package profiling

import (
	"net/http"
	"testing"
	"time"

	"github.com/tos-network/tos-pool/internal/config"
)

func TestNewServer(t *testing.T) {
	cfg := &config.ProfilingConfig{
		Enabled: true,
		Bind:    "127.0.0.1:6060",
	}

	server := NewServer(cfg)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.cfg != cfg {
		t.Error("Server.cfg not set correctly")
	}

	if server.server != nil {
		t.Error("Server.server should be nil before Start()")
	}
}

func TestServerStartDisabled(t *testing.T) {
	cfg := &config.ProfilingConfig{
		Enabled: false,
		Bind:    "127.0.0.1:6060",
	}

	server := NewServer(cfg)

	err := server.Start()
	if err != nil {
		t.Errorf("Start() returned error when disabled: %v", err)
	}

	// Server should not be created when disabled
	if server.server != nil {
		t.Error("Server.server should be nil when disabled")
	}
}

func TestServerStartEnabled(t *testing.T) {
	cfg := &config.ProfilingConfig{
		Enabled: true,
		Bind:    "127.0.0.1:0", // Use port 0 to get random available port
	}

	server := NewServer(cfg)

	err := server.Start()
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer server.Stop()

	// Server should be created
	if server.server == nil {
		t.Error("Server.server should not be nil after Start()")
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)
}

func TestServerStop(t *testing.T) {
	cfg := &config.ProfilingConfig{
		Enabled: true,
		Bind:    "127.0.0.1:0",
	}

	server := NewServer(cfg)

	err := server.Start()
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	err = server.Stop()
	if err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}

func TestServerStopNotStarted(t *testing.T) {
	cfg := &config.ProfilingConfig{
		Enabled: true,
		Bind:    "127.0.0.1:6060",
	}

	server := NewServer(cfg)

	// Stop without starting should not error
	err := server.Stop()
	if err != nil {
		t.Errorf("Stop() on unstarted server returned error: %v", err)
	}
}

func TestProfilingEndpoints(t *testing.T) {
	cfg := &config.ProfilingConfig{
		Enabled: true,
		Bind:    "127.0.0.1:16060", // Use specific port for test
	}

	server := NewServer(cfg)

	err := server.Start()
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer server.Stop()

	time.Sleep(200 * time.Millisecond)

	// Test pprof index endpoint
	endpoints := []struct {
		path   string
		method string
	}{
		{"/debug/pprof/", "GET"},
		{"/debug/pprof/goroutine", "GET"},
		{"/debug/pprof/heap", "GET"},
		{"/debug/pprof/allocs", "GET"},
		{"/debug/pprof/threadcreate", "GET"},
		{"/debug/pprof/block", "GET"},
		{"/debug/pprof/mutex", "GET"},
		{"/debug/pprof/cmdline", "GET"},
		{"/debug/pprof/symbol", "POST"},
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, ep := range endpoints {
		url := "http://127.0.0.1:16060" + ep.path
		var resp *http.Response
		var err error

		if ep.method == "POST" {
			resp, err = client.Post(url, "text/plain", nil)
		} else {
			resp, err = client.Get(url)
		}

		if err != nil {
			t.Errorf("Request to %s failed: %v", ep.path, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Endpoint %s returned status %d, want 200", ep.path, resp.StatusCode)
		}

		resp.Body.Close()
	}
}

func TestServerMultipleStartStop(t *testing.T) {
	cfg := &config.ProfilingConfig{
		Enabled: true,
		Bind:    "127.0.0.1:0",
	}

	server := NewServer(cfg)

	// First start/stop
	if err := server.Start(); err != nil {
		t.Fatalf("First Start() failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := server.Stop(); err != nil {
		t.Errorf("First Stop() failed: %v", err)
	}

	// Second start/stop - should work
	server2 := NewServer(cfg)
	if err := server2.Start(); err != nil {
		t.Fatalf("Second Start() failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := server2.Stop(); err != nil {
		t.Errorf("Second Stop() failed: %v", err)
	}
}

func TestProfilingConfigStruct(t *testing.T) {
	cfg := config.ProfilingConfig{
		Enabled: true,
		Bind:    "0.0.0.0:6060",
	}

	if !cfg.Enabled {
		t.Error("ProfilingConfig.Enabled should be true")
	}

	if cfg.Bind != "0.0.0.0:6060" {
		t.Errorf("ProfilingConfig.Bind = %s, want 0.0.0.0:6060", cfg.Bind)
	}
}

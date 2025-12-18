package newrelic

import (
	"context"
	"testing"

	"github.com/tos-network/tos-pool/internal/config"
)

func TestNewAgent(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled:    true,
		AppName:    "Test Pool",
		LicenseKey: "test_key",
	}

	agent := NewAgent(cfg)

	if agent == nil {
		t.Fatal("NewAgent returned nil")
	}

	if agent.cfg != cfg {
		t.Error("Agent.cfg not set correctly")
	}

	if agent.app != nil {
		t.Error("Agent.app should be nil before Start()")
	}
}

func TestStartDisabled(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)
	err := agent.Start()

	if err != nil {
		t.Errorf("Start() returned error when disabled: %v", err)
	}

	if agent.app != nil {
		t.Error("Agent.app should be nil when disabled")
	}
}

func TestStartNoLicenseKey(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled:    true,
		AppName:    "Test Pool",
		LicenseKey: "",
	}

	agent := NewAgent(cfg)
	err := agent.Start()

	if err != nil {
		t.Errorf("Start() returned error with empty license key: %v", err)
	}

	if agent.app != nil {
		t.Error("Agent.app should be nil with empty license key")
	}
}

func TestStopNotStarted(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic
	agent.Stop()
}

func TestApplicationNotStarted(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	app := agent.Application()
	if app != nil {
		t.Error("Application() should return nil when not started")
	}
}

func TestIsEnabledNotStarted(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	if agent.IsEnabled() {
		t.Error("IsEnabled() should return false when not started")
	}
}

func TestStartTransactionNotStarted(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	txn := agent.StartTransaction("test")
	if txn != nil {
		t.Error("StartTransaction() should return nil when not started")
	}
}

func TestRecordCustomEventNotStarted(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic
	agent.RecordCustomEvent("TestEvent", map[string]interface{}{
		"key": "value",
	})
}

func TestRecordCustomMetricNotStarted(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic
	agent.RecordCustomMetric("Custom/Test", 123.45)
}

func TestNoticeErrorNilTransaction(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic with nil transaction
	agent.NoticeError(nil, nil)
}

func TestNewContextNilTransaction(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)
	ctx := context.Background()

	result := agent.NewContext(ctx, nil)
	if result != ctx {
		t.Error("NewContext should return original context when txn is nil")
	}
}

func TestFromContext(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)
	ctx := context.Background()

	txn := agent.FromContext(ctx)
	if txn != nil {
		t.Error("FromContext should return nil for empty context")
	}
}

func TestRecordShareSubmission(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic when not started
	agent.RecordShareSubmission("tos1address", "worker1", 1000000, true)
	agent.RecordShareSubmission("tos1address", "worker1", 1000000, false)
}

func TestRecordBlockFound(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic when not started
	agent.RecordBlockFound(12345, "tos1finder", 5000000000)
}

func TestRecordPayment(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic when not started
	agent.RecordPayment("tos1address", 1000000000, "0xhash")
}

func TestRecordMinerConnected(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic when not started
	agent.RecordMinerConnected("tos1address", "worker1", "192.168.1.100")
}

func TestRecordMinerDisconnected(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic when not started
	agent.RecordMinerDisconnected("tos1address", "worker1")
}

func TestUpdatePoolMetrics(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic when not started
	agent.UpdatePoolMetrics(1500000.5, 100, 250)
}

func TestUpdateNetworkMetrics(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Should not panic when not started
	agent.UpdateNetworkMetrics(12345, 1000000, 5000000.5)
}

func TestAgentStructFields(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled:    true,
		AppName:    "TOS Pool",
		LicenseKey: "license_123",
	}

	agent := NewAgent(cfg)

	if agent.cfg.AppName != "TOS Pool" {
		t.Errorf("AppName = %s, want TOS Pool", agent.cfg.AppName)
	}

	if agent.cfg.LicenseKey != "license_123" {
		t.Errorf("LicenseKey = %s, want license_123", agent.cfg.LicenseKey)
	}
}

func TestConcurrentAccess(t *testing.T) {
	cfg := &config.NewRelicConfig{
		Enabled: false,
	}

	agent := NewAgent(cfg)

	// Test concurrent access - should not panic
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			agent.IsEnabled()
			agent.Application()
			agent.StartTransaction("test")
			agent.RecordCustomEvent("test", nil)
			agent.RecordCustomMetric("test", 1.0)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

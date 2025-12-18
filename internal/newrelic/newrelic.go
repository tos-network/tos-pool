// Package newrelic provides New Relic APM integration for monitoring.
package newrelic

import (
	"context"
	"sync"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/util"
)

// Agent wraps New Relic APM functionality
type Agent struct {
	cfg   *config.NewRelicConfig
	app   *newrelic.Application
	mu    sync.RWMutex
}

// NewAgent creates a new New Relic agent
func NewAgent(cfg *config.NewRelicConfig) *Agent {
	return &Agent{
		cfg: cfg,
	}
}

// Start initializes the New Relic agent
func (a *Agent) Start() error {
	if !a.cfg.Enabled {
		util.Info("New Relic APM disabled")
		return nil
	}

	if a.cfg.LicenseKey == "" {
		util.Warn("New Relic license key not configured, APM disabled")
		return nil
	}

	app, err := newrelic.NewApplication(
		newrelic.ConfigAppName(a.cfg.AppName),
		newrelic.ConfigLicense(a.cfg.LicenseKey),
		newrelic.ConfigDistributedTracerEnabled(true),
		newrelic.ConfigAppLogForwardingEnabled(true),
	)
	if err != nil {
		return err
	}

	// Wait for connection (up to 5 seconds)
	if err := app.WaitForConnection(5 * time.Second); err != nil {
		util.Warnf("New Relic connection timeout: %v (will retry in background)", err)
	}

	a.mu.Lock()
	a.app = app
	a.mu.Unlock()

	util.Infof("New Relic APM enabled for app: %s", a.cfg.AppName)
	return nil
}

// Stop shuts down the New Relic agent
func (a *Agent) Stop() {
	a.mu.RLock()
	app := a.app
	a.mu.RUnlock()

	if app != nil {
		util.Info("Shutting down New Relic agent")
		app.Shutdown(10 * time.Second)
	}
}

// Application returns the underlying New Relic application (for middleware)
func (a *Agent) Application() *newrelic.Application {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.app
}

// IsEnabled returns true if New Relic is enabled and connected
func (a *Agent) IsEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.app != nil
}

// StartTransaction starts a new New Relic transaction
func (a *Agent) StartTransaction(name string) *newrelic.Transaction {
	a.mu.RLock()
	app := a.app
	a.mu.RUnlock()

	if app == nil {
		return nil
	}
	return app.StartTransaction(name)
}

// RecordCustomEvent records a custom event
func (a *Agent) RecordCustomEvent(eventType string, params map[string]interface{}) {
	a.mu.RLock()
	app := a.app
	a.mu.RUnlock()

	if app != nil {
		app.RecordCustomEvent(eventType, params)
	}
}

// RecordCustomMetric records a custom metric
func (a *Agent) RecordCustomMetric(name string, value float64) {
	a.mu.RLock()
	app := a.app
	a.mu.RUnlock()

	if app != nil {
		app.RecordCustomMetric(name, value)
	}
}

// NoticeError records an error
func (a *Agent) NoticeError(txn *newrelic.Transaction, err error) {
	if txn != nil && err != nil {
		txn.NoticeError(err)
	}
}

// NewContext adds transaction to context
func (a *Agent) NewContext(ctx context.Context, txn *newrelic.Transaction) context.Context {
	if txn == nil {
		return ctx
	}
	return newrelic.NewContext(ctx, txn)
}

// FromContext gets transaction from context
func (a *Agent) FromContext(ctx context.Context) *newrelic.Transaction {
	return newrelic.FromContext(ctx)
}

// RecordShareSubmission records a share submission event
func (a *Agent) RecordShareSubmission(address, worker string, difficulty uint64, valid bool) {
	status := "valid"
	if !valid {
		status = "invalid"
	}
	a.RecordCustomEvent("ShareSubmission", map[string]interface{}{
		"address":    address,
		"worker":     worker,
		"difficulty": difficulty,
		"status":     status,
	})
}

// RecordBlockFound records a block found event
func (a *Agent) RecordBlockFound(height uint64, finder string, reward uint64) {
	a.RecordCustomEvent("BlockFound", map[string]interface{}{
		"height": height,
		"finder": finder,
		"reward": reward,
	})
}

// RecordPayment records a payment event
func (a *Agent) RecordPayment(address string, amount uint64, txHash string) {
	a.RecordCustomEvent("Payment", map[string]interface{}{
		"address": address,
		"amount":  amount,
		"txHash":  txHash,
	})
}

// RecordMinerConnected records a miner connection
func (a *Agent) RecordMinerConnected(address, worker, ip string) {
	a.RecordCustomEvent("MinerConnected", map[string]interface{}{
		"address": address,
		"worker":  worker,
		"ip":      ip,
	})
}

// RecordMinerDisconnected records a miner disconnection
func (a *Agent) RecordMinerDisconnected(address, worker string) {
	a.RecordCustomEvent("MinerDisconnected", map[string]interface{}{
		"address": address,
		"worker":  worker,
	})
}

// UpdatePoolMetrics updates pool-wide metrics
func (a *Agent) UpdatePoolMetrics(hashrate float64, miners, workers int64) {
	a.RecordCustomMetric("Custom/Pool/Hashrate", hashrate)
	a.RecordCustomMetric("Custom/Pool/Miners", float64(miners))
	a.RecordCustomMetric("Custom/Pool/Workers", float64(workers))
}

// UpdateNetworkMetrics updates network metrics
func (a *Agent) UpdateNetworkMetrics(height uint64, difficulty uint64, hashrate float64) {
	a.RecordCustomMetric("Custom/Network/Height", float64(height))
	a.RecordCustomMetric("Custom/Network/Difficulty", float64(difficulty))
	a.RecordCustomMetric("Custom/Network/Hashrate", hashrate)
}

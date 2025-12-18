package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        1.0,
					FeeAddress: "tos1testaddress",
				},
				Node: NodeConfig{
					URL:     "http://127.0.0.1:8545",
					Timeout: 10 * time.Second,
				},
				Master: MasterConfig{
					Enabled: true,
					Secret:  "test-secret",
				},
				Mining: MiningConfig{
					MinDifficulty:     1000,
					MaxDifficulty:     1000000,
					VardiffTargetTime: 4.0,
				},
				Payouts: PayoutsConfig{
					Threshold: 100000000,
				},
			},
			wantErr: false,
		},
		{
			name: "missing fee address",
			config: Config{
				Pool: PoolConfig{
					Name: "Test Pool",
					Fee:  1.0,
				},
			},
			wantErr: true,
			errMsg:  "pool.fee_address is required",
		},
		{
			name: "invalid fee - negative",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        -1.0,
					FeeAddress: "tos1test",
				},
			},
			wantErr: true,
			errMsg:  "pool.fee must be between 0 and 100",
		},
		{
			name: "invalid fee - over 100",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        101.0,
					FeeAddress: "tos1test",
				},
			},
			wantErr: true,
			errMsg:  "pool.fee must be between 0 and 100",
		},
		{
			name: "missing node url",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        1.0,
					FeeAddress: "tos1test",
				},
				Node: NodeConfig{},
			},
			wantErr: true,
			errMsg:  "node.url or node.upstreams is required",
		},
		{
			name: "missing master secret",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        1.0,
					FeeAddress: "tos1test",
				},
				Node: NodeConfig{
					URL: "http://127.0.0.1:8545",
				},
				Master: MasterConfig{
					Enabled: true,
					Secret:  "",
				},
			},
			wantErr: true,
			errMsg:  "master.secret is required when master is enabled",
		},
		{
			name: "invalid difficulty range",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        1.0,
					FeeAddress: "tos1test",
				},
				Node: NodeConfig{
					URL: "http://127.0.0.1:8545",
				},
				Master: MasterConfig{
					Enabled: false,
				},
				Mining: MiningConfig{
					MinDifficulty: 1000000,
					MaxDifficulty: 1000,
				},
			},
			wantErr: true,
			errMsg:  "mining.min_difficulty must be <= max_difficulty",
		},
		{
			name: "invalid vardiff target time",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        1.0,
					FeeAddress: "tos1test",
				},
				Node: NodeConfig{
					URL: "http://127.0.0.1:8545",
				},
				Master: MasterConfig{
					Enabled: false,
				},
				Mining: MiningConfig{
					MinDifficulty:     1000,
					MaxDifficulty:     1000000,
					VardiffTargetTime: 0,
				},
			},
			wantErr: true,
			errMsg:  "mining.vardiff_target_time must be positive",
		},
		{
			name: "missing payout threshold",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        1.0,
					FeeAddress: "tos1test",
				},
				Node: NodeConfig{
					URL: "http://127.0.0.1:8545",
				},
				Master: MasterConfig{
					Enabled: false,
				},
				Mining: MiningConfig{
					MinDifficulty:     1000,
					MaxDifficulty:     1000000,
					VardiffTargetTime: 4.0,
				},
				Payouts: PayoutsConfig{
					Threshold: 0,
				},
			},
			wantErr: true,
			errMsg:  "payouts.threshold must be > 0",
		},
		{
			name: "upstream with empty url",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        1.0,
					FeeAddress: "tos1test",
				},
				Node: NodeConfig{
					Upstreams: []UpstreamConfig{
						{Name: "test", URL: ""},
					},
				},
			},
			wantErr: true,
			errMsg:  "node.upstreams[0].url is required",
		},
		{
			name: "valid config with upstreams",
			config: Config{
				Pool: PoolConfig{
					Name:       "Test Pool",
					Fee:        1.0,
					FeeAddress: "tos1test",
				},
				Node: NodeConfig{
					Upstreams: []UpstreamConfig{
						{Name: "primary", URL: "http://127.0.0.1:8545"},
						{Name: "backup", URL: "http://127.0.0.2:8545"},
					},
				},
				Master: MasterConfig{
					Enabled: false,
				},
				Mining: MiningConfig{
					MinDifficulty:     1000,
					MaxDifficulty:     1000000,
					VardiffTargetTime: 4.0,
				},
				Payouts: PayoutsConfig{
					Threshold: 100000000,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestIsCombinedMode(t *testing.T) {
	tests := []struct {
		name     string
		master   bool
		slave    bool
		expected bool
	}{
		{"both enabled", true, true, true},
		{"master only", true, false, false},
		{"slave only", false, true, false},
		{"both disabled", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Master: MasterConfig{Enabled: tt.master},
				Slave:  SlaveConfig{Enabled: tt.slave},
			}
			if got := cfg.IsCombinedMode(); got != tt.expected {
				t.Errorf("IsCombinedMode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsMasterOnly(t *testing.T) {
	tests := []struct {
		name     string
		master   bool
		slave    bool
		expected bool
	}{
		{"both enabled", true, true, false},
		{"master only", true, false, true},
		{"slave only", false, true, false},
		{"both disabled", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Master: MasterConfig{Enabled: tt.master},
				Slave:  SlaveConfig{Enabled: tt.slave},
			}
			if got := cfg.IsMasterOnly(); got != tt.expected {
				t.Errorf("IsMasterOnly() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsSlaveOnly(t *testing.T) {
	tests := []struct {
		name     string
		master   bool
		slave    bool
		expected bool
	}{
		{"both enabled", true, true, false},
		{"master only", true, false, false},
		{"slave only", false, true, true},
		{"both disabled", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Master: MasterConfig{Enabled: tt.master},
				Slave:  SlaveConfig{Enabled: tt.slave},
			}
			if got := cfg.IsSlaveOnly(); got != tt.expected {
				t.Errorf("IsSlaveOnly() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfigStructs(t *testing.T) {
	// Test PoolConfig
	pool := PoolConfig{
		Name:       "Test Pool",
		Fee:        1.5,
		FeeAddress: "tos1test",
	}
	if pool.Name != "Test Pool" {
		t.Errorf("PoolConfig.Name = %s, want Test Pool", pool.Name)
	}
	if pool.Fee != 1.5 {
		t.Errorf("PoolConfig.Fee = %f, want 1.5", pool.Fee)
	}

	// Test NodeConfig
	node := NodeConfig{
		URL:                 "http://localhost:8545",
		Timeout:             10 * time.Second,
		HealthCheckInterval: 5 * time.Second,
		MaxFailures:         3,
	}
	if node.URL != "http://localhost:8545" {
		t.Errorf("NodeConfig.URL = %s, want http://localhost:8545", node.URL)
	}
	if node.MaxFailures != 3 {
		t.Errorf("NodeConfig.MaxFailures = %d, want 3", node.MaxFailures)
	}

	// Test UpstreamConfig
	upstream := UpstreamConfig{
		Name:    "primary",
		URL:     "http://127.0.0.1:8545",
		Timeout: 10 * time.Second,
		Weight:  10,
	}
	if upstream.Weight != 10 {
		t.Errorf("UpstreamConfig.Weight = %d, want 10", upstream.Weight)
	}

	// Test RedisConfig
	redis := RedisConfig{
		URL:      "localhost:6379",
		Password: "secret",
		DB:       1,
	}
	if redis.DB != 1 {
		t.Errorf("RedisConfig.DB = %d, want 1", redis.DB)
	}

	// Test MiningConfig
	mining := MiningConfig{
		InitialDifficulty:  1000000,
		MinDifficulty:      1000,
		MaxDifficulty:      1000000000,
		VardiffTargetTime:  4.0,
		VardiffRetarget:    90.0,
		VardiffVariance:    30.0,
		JobRefreshInterval: 500 * time.Millisecond,
	}
	if mining.InitialDifficulty != 1000000 {
		t.Errorf("MiningConfig.InitialDifficulty = %d, want 1000000", mining.InitialDifficulty)
	}

	// Test ValidationConfig
	validation := ValidationConfig{
		TrustThreshold:      50,
		TrustCheckPercent:   75,
		HashrateWindow:      10 * time.Minute,
		HashrateLargeWindow: 3 * time.Hour,
	}
	if validation.TrustThreshold != 50 {
		t.Errorf("ValidationConfig.TrustThreshold = %d, want 50", validation.TrustThreshold)
	}

	// Test UnlockerConfig
	unlocker := UnlockerConfig{
		Enabled:       true,
		Interval:      60 * time.Second,
		ImmatureDepth: 10,
		MatureDepth:   100,
	}
	if unlocker.MatureDepth != 100 {
		t.Errorf("UnlockerConfig.MatureDepth = %d, want 100", unlocker.MatureDepth)
	}

	// Test PPLNSConfig
	pplns := PPLNSConfig{
		Window:        2.0,
		DynamicWindow: true,
		MinWindow:     0.5,
		MaxWindow:     4.0,
	}
	if !pplns.DynamicWindow {
		t.Error("PPLNSConfig.DynamicWindow should be true")
	}

	// Test PayoutsConfig
	payouts := PayoutsConfig{
		Enabled:           true,
		Interval:          1 * time.Hour,
		Threshold:         100000000,
		MaxAddressesPerTx: 100,
		GasLimit:          21000,
		GasPrice:          "auto",
		WithdrawalFee:     1000,
		WithdrawalFeeRate: 0.01,
	}
	if payouts.WithdrawalFeeRate != 0.01 {
		t.Errorf("PayoutsConfig.WithdrawalFeeRate = %f, want 0.01", payouts.WithdrawalFeeRate)
	}

	// Test APIConfig
	api := APIConfig{
		Enabled:       true,
		Bind:          "0.0.0.0:8080",
		StatsCache:    10 * time.Second,
		CORSOrigins:   []string{"*"},
		AdminEnabled:  true,
		AdminPassword: "admin123",
	}
	if !api.AdminEnabled {
		t.Error("APIConfig.AdminEnabled should be true")
	}

	// Test SecurityConfig
	security := SecurityConfig{
		MaxConnectionsPerIP:  100,
		MaxWorkersPerAddress: 256,
		BanThreshold:         30,
		BanDuration:          1 * time.Hour,
		RateLimitShares:      100,
	}
	if security.MaxConnectionsPerIP != 100 {
		t.Errorf("SecurityConfig.MaxConnectionsPerIP = %d, want 100", security.MaxConnectionsPerIP)
	}

	// Test NotifyConfig
	notify := NotifyConfig{
		Enabled:      true,
		DiscordURL:   "https://discord.com/api/webhooks/...",
		TelegramBot:  "bot_token",
		TelegramChat: "chat_id",
		PoolURL:      "https://pool.example.com",
	}
	if !notify.Enabled {
		t.Error("NotifyConfig.Enabled should be true")
	}

	// Test LogConfig
	log := LogConfig{
		Level:  "debug",
		Format: "json",
		File:   "/var/log/pool.log",
	}
	if log.Level != "debug" {
		t.Errorf("LogConfig.Level = %s, want debug", log.Level)
	}

	// Test ProfilingConfig
	profiling := ProfilingConfig{
		Enabled: true,
		Bind:    "127.0.0.1:6060",
	}
	if !profiling.Enabled {
		t.Error("ProfilingConfig.Enabled should be true")
	}

	// Test NewRelicConfig
	newrelic := NewRelicConfig{
		Enabled:    true,
		AppName:    "TOS Pool",
		LicenseKey: "license_key_here",
	}
	if newrelic.AppName != "TOS Pool" {
		t.Errorf("NewRelicConfig.AppName = %s, want TOS Pool", newrelic.AppName)
	}
}

func TestLoadWithTempConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
pool:
  name: "Test Pool"
  fee: 1.0
  fee_address: "tos1testaddress"

node:
  url: "http://127.0.0.1:8545"
  timeout: 10s

master:
  enabled: false

slave:
  enabled: true
  stratum_bind: "0.0.0.0:3333"

mining:
  initial_difficulty: 1000000
  min_difficulty: 1000
  max_difficulty: 1000000000
  vardiff_target_time: 4.0

payouts:
  threshold: 100000000
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Pool.Name != "Test Pool" {
		t.Errorf("Pool.Name = %s, want Test Pool", cfg.Pool.Name)
	}

	if cfg.Pool.Fee != 1.0 {
		t.Errorf("Pool.Fee = %f, want 1.0", cfg.Pool.Fee)
	}

	if cfg.Node.URL != "http://127.0.0.1:8545" {
		t.Errorf("Node.URL = %s, want http://127.0.0.1:8545", cfg.Node.URL)
	}

	if cfg.Master.Enabled {
		t.Error("Master.Enabled should be false")
	}

	if !cfg.Slave.Enabled {
		t.Error("Slave.Enabled should be true")
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	// Create temp config file with invalid content
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Missing required fee_address
	configContent := `
pool:
  name: "Test Pool"
  fee: 1.0

node:
  url: "http://127.0.0.1:8545"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() should return error for invalid config")
	}
}

func TestLoadNonexistentConfig(t *testing.T) {
	// Try to load from non-existent path without default config
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() should return error for non-existent config")
	}
}

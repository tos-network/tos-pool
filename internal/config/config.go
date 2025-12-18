// Package config handles configuration loading and validation for TOS Pool.
package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the pool
type Config struct {
	Pool       PoolConfig       `mapstructure:"pool"`
	Node       NodeConfig       `mapstructure:"node"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Master     MasterConfig     `mapstructure:"master"`
	Slave      SlaveConfig      `mapstructure:"slave"`
	Mining     MiningConfig     `mapstructure:"mining"`
	Validation ValidationConfig `mapstructure:"validation"`
	Unlocker   UnlockerConfig   `mapstructure:"unlocker"`
	PPLNS      PPLNSConfig      `mapstructure:"pplns"`
	Payouts    PayoutsConfig    `mapstructure:"payouts"`
	API        APIConfig        `mapstructure:"api"`
	Security   SecurityConfig   `mapstructure:"security"`
	Notify     NotifyConfig     `mapstructure:"notify"`
	Log        LogConfig        `mapstructure:"log"`
	Profiling  ProfilingConfig  `mapstructure:"profiling"`
	NewRelic   NewRelicConfig   `mapstructure:"newrelic"`
}

// PoolConfig defines pool identity settings
type PoolConfig struct {
	Name       string  `mapstructure:"name"`
	Fee        float64 `mapstructure:"fee"`
	FeeAddress string  `mapstructure:"fee_address"`
}

// NodeConfig defines TOS node connection settings
type NodeConfig struct {
	URL     string        `mapstructure:"url"`      // Primary node URL (for backward compatibility)
	Timeout time.Duration `mapstructure:"timeout"`

	// Multi-upstream failover settings
	Upstreams           []UpstreamConfig `mapstructure:"upstreams"`            // Multiple upstream nodes
	HealthCheckInterval time.Duration    `mapstructure:"health_check_interval"` // Health check interval (default 5s)
	HealthCheckTimeout  time.Duration    `mapstructure:"health_check_timeout"`  // Health check timeout (default 3s)
	MaxFailures         int              `mapstructure:"max_failures"`          // Failures before marking unhealthy (default 3)
	RecoveryThreshold   int              `mapstructure:"recovery_threshold"`    // Successes before marking healthy (default 2)
}

// UpstreamConfig defines settings for a single upstream node
type UpstreamConfig struct {
	Name    string        `mapstructure:"name"`    // Friendly name for logging
	URL     string        `mapstructure:"url"`     // Node RPC URL
	Timeout time.Duration `mapstructure:"timeout"` // Per-upstream timeout (optional, uses global if not set)
	Weight  int           `mapstructure:"weight"`  // Priority weight (higher = preferred, default 1)
}

// RedisConfig defines Redis connection settings
type RedisConfig struct {
	URL      string `mapstructure:"url"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// MasterConfig defines master server settings
type MasterConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Bind    string `mapstructure:"bind"`
	Secret  string `mapstructure:"secret"`
}

// SlaveConfig defines slave server settings
type SlaveConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	MasterURL      string `mapstructure:"master_url"`
	StratumBind    string `mapstructure:"stratum_bind"`
	StratumTLSBind string `mapstructure:"stratum_tls_bind"`
	TLSCert        string `mapstructure:"tls_cert"`
	TLSKey         string `mapstructure:"tls_key"`
	GetworkEnabled bool   `mapstructure:"getwork_enabled"`
	GetworkBind    string `mapstructure:"getwork_bind"`

	// WebSocket GetWork settings
	WebSocketEnabled bool   `mapstructure:"websocket_enabled"`
	WebSocketBind    string `mapstructure:"websocket_bind"`

	// Xatum protocol settings (TLS-secured JSON mining protocol)
	XatumEnabled bool   `mapstructure:"xatum_enabled"`
	XatumBind    string `mapstructure:"xatum_bind"`
}

// MiningConfig defines mining difficulty settings
type MiningConfig struct {
	InitialDifficulty  uint64        `mapstructure:"initial_difficulty"`
	MinDifficulty      uint64        `mapstructure:"min_difficulty"`
	MaxDifficulty      uint64        `mapstructure:"max_difficulty"`
	VardiffTargetTime  float64       `mapstructure:"vardiff_target_time"`
	VardiffRetarget    float64       `mapstructure:"vardiff_retarget"`
	VardiffVariance    float64       `mapstructure:"vardiff_variance"`
	JobRefreshInterval time.Duration `mapstructure:"job_refresh_interval"`
}

// ValidationConfig defines share validation settings
type ValidationConfig struct {
	TrustThreshold     int           `mapstructure:"trust_threshold"`
	TrustCheckPercent  int           `mapstructure:"trust_check_percent"`
	HashrateWindow     time.Duration `mapstructure:"hashrate_window"`
	HashrateLargeWindow time.Duration `mapstructure:"hashrate_large_window"`
}

// UnlockerConfig defines block unlocking settings
type UnlockerConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	Interval     time.Duration `mapstructure:"interval"`
	ImmatureDepth uint64       `mapstructure:"immature_depth"`
	MatureDepth   uint64       `mapstructure:"mature_depth"`
}

// PPLNSConfig defines PPLNS payment settings
type PPLNSConfig struct {
	Window        float64 `mapstructure:"window"`
	DynamicWindow bool    `mapstructure:"dynamic_window"`
	MinWindow     float64 `mapstructure:"min_window"`
	MaxWindow     float64 `mapstructure:"max_window"`
}

// NotifyConfig defines notification settings
type NotifyConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	DiscordURL   string `mapstructure:"discord_url"`
	TelegramBot  string `mapstructure:"telegram_bot"`
	TelegramChat string `mapstructure:"telegram_chat"`
	PoolURL      string `mapstructure:"pool_url"`
}

// PayoutsConfig defines payment processing settings
type PayoutsConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	Interval          time.Duration `mapstructure:"interval"`
	Threshold         uint64        `mapstructure:"threshold"`
	MaxAddressesPerTx int           `mapstructure:"max_addresses_per_tx"`
	GasLimit          uint64        `mapstructure:"gas_limit"`
	GasPrice          string        `mapstructure:"gas_price"`
	WithdrawalFee     uint64        `mapstructure:"withdrawal_fee"`      // Fixed fee per withdrawal (in smallest unit)
	WithdrawalFeeRate float64       `mapstructure:"withdrawal_fee_rate"` // Percentage fee (0.01 = 1%)
}

// APIConfig defines API server settings
type APIConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	Bind          string        `mapstructure:"bind"`
	StatsCache    time.Duration `mapstructure:"stats_cache"`
	CORSOrigins   []string      `mapstructure:"cors_origins"`
	AdminEnabled  bool          `mapstructure:"admin_enabled"`
	AdminPassword string        `mapstructure:"admin_password"`
}

// SecurityConfig defines security settings
type SecurityConfig struct {
	MaxConnectionsPerIP  int           `mapstructure:"max_connections_per_ip"`
	MaxWorkersPerAddress int           `mapstructure:"max_workers_per_address"`
	BanThreshold         int           `mapstructure:"ban_threshold"`
	BanDuration          time.Duration `mapstructure:"ban_duration"`
	RateLimitShares      int           `mapstructure:"rate_limit_shares"`
}

// LogConfig defines logging settings
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	File   string `mapstructure:"file"`
}

// ProfilingConfig defines pprof profiling settings
type ProfilingConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Bind    string `mapstructure:"bind"`
}

// NewRelicConfig defines New Relic APM settings
type NewRelicConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	AppName    string `mapstructure:"app_name"`
	LicenseKey string `mapstructure:"license_key"`
}

// Load reads configuration from file and environment
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/tos-pool")
	}

	// Read environment variables
	v.SetEnvPrefix("TOS_POOL")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Pool defaults
	v.SetDefault("pool.name", "TOS Mining Pool")
	v.SetDefault("pool.fee", 1.0)

	// Node defaults
	v.SetDefault("node.url", "http://127.0.0.1:8545")
	v.SetDefault("node.timeout", "10s")
	v.SetDefault("node.health_check_interval", "5s")
	v.SetDefault("node.health_check_timeout", "3s")
	v.SetDefault("node.max_failures", 3)
	v.SetDefault("node.recovery_threshold", 2)

	// Redis defaults
	v.SetDefault("redis.url", "127.0.0.1:6379")
	v.SetDefault("redis.db", 0)

	// Master defaults
	v.SetDefault("master.enabled", true)
	v.SetDefault("master.bind", "0.0.0.0:3221")

	// Slave defaults
	v.SetDefault("slave.enabled", true)
	v.SetDefault("slave.stratum_bind", "0.0.0.0:3333")
	v.SetDefault("slave.stratum_tls_bind", "0.0.0.0:3334")
	v.SetDefault("slave.getwork_enabled", false)
	v.SetDefault("slave.getwork_bind", "0.0.0.0:8888")
	v.SetDefault("slave.websocket_enabled", false)
	v.SetDefault("slave.websocket_bind", "0.0.0.0:3335")
	v.SetDefault("slave.xatum_enabled", false)
	v.SetDefault("slave.xatum_bind", "0.0.0.0:3336")

	// Mining defaults
	v.SetDefault("mining.initial_difficulty", 1000000)
	v.SetDefault("mining.min_difficulty", 1000)
	v.SetDefault("mining.max_difficulty", 1000000000000)
	v.SetDefault("mining.vardiff_target_time", 4.0)
	v.SetDefault("mining.vardiff_retarget", 90.0)
	v.SetDefault("mining.vardiff_variance", 30.0)
	v.SetDefault("mining.job_refresh_interval", "500ms")

	// Validation defaults
	v.SetDefault("validation.trust_threshold", 50)
	v.SetDefault("validation.trust_check_percent", 75)
	v.SetDefault("validation.hashrate_window", "10m")
	v.SetDefault("validation.hashrate_large_window", "3h")

	// Unlocker defaults
	v.SetDefault("unlocker.enabled", true)
	v.SetDefault("unlocker.interval", "60s")
	v.SetDefault("unlocker.immature_depth", 10)
	v.SetDefault("unlocker.mature_depth", 100)

	// PPLNS defaults
	v.SetDefault("pplns.window", 2.0)
	v.SetDefault("pplns.dynamic_window", false)
	v.SetDefault("pplns.min_window", 0.5)
	v.SetDefault("pplns.max_window", 4.0)

	// Payouts defaults
	v.SetDefault("payouts.enabled", true)
	v.SetDefault("payouts.interval", "1h")
	v.SetDefault("payouts.threshold", 100000000) // 0.1 TOS
	v.SetDefault("payouts.max_addresses_per_tx", 100)
	v.SetDefault("payouts.gas_limit", 21000)
	v.SetDefault("payouts.gas_price", "auto")
	v.SetDefault("payouts.withdrawal_fee", 0)        // No fixed fee by default
	v.SetDefault("payouts.withdrawal_fee_rate", 0.0) // No percentage fee by default

	// API defaults
	v.SetDefault("api.enabled", true)
	v.SetDefault("api.bind", "0.0.0.0:8080")
	v.SetDefault("api.stats_cache", "10s")
	v.SetDefault("api.cors_origins", []string{"*"})
	v.SetDefault("api.admin_enabled", false)
	v.SetDefault("api.admin_password", "")

	// Notify defaults
	v.SetDefault("notify.enabled", false)
	v.SetDefault("notify.discord_url", "")
	v.SetDefault("notify.telegram_bot", "")
	v.SetDefault("notify.telegram_chat", "")
	v.SetDefault("notify.pool_url", "")

	// Security defaults
	v.SetDefault("security.max_connections_per_ip", 100)
	v.SetDefault("security.max_workers_per_address", 256)
	v.SetDefault("security.ban_threshold", 30)
	v.SetDefault("security.ban_duration", "1h")
	v.SetDefault("security.rate_limit_shares", 100)

	// Log defaults
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "console")

	// Profiling defaults (pprof)
	v.SetDefault("profiling.enabled", false)
	v.SetDefault("profiling.bind", "127.0.0.1:6060")

	// New Relic defaults
	v.SetDefault("newrelic.enabled", false)
	v.SetDefault("newrelic.app_name", "TOS Pool")
	v.SetDefault("newrelic.license_key", "")
}

// Validate checks configuration for errors
func (c *Config) Validate() error {
	if c.Pool.FeeAddress == "" {
		return fmt.Errorf("pool.fee_address is required")
	}

	if c.Pool.Fee < 0 || c.Pool.Fee > 100 {
		return fmt.Errorf("pool.fee must be between 0 and 100")
	}

	// Either node.url or node.upstreams must be configured
	if c.Node.URL == "" && len(c.Node.Upstreams) == 0 {
		return fmt.Errorf("node.url or node.upstreams is required")
	}

	// Validate upstream configs
	for i, upstream := range c.Node.Upstreams {
		if upstream.URL == "" {
			return fmt.Errorf("node.upstreams[%d].url is required", i)
		}
	}

	if c.Master.Enabled && c.Master.Secret == "" {
		return fmt.Errorf("master.secret is required when master is enabled")
	}

	if c.Mining.MinDifficulty > c.Mining.MaxDifficulty {
		return fmt.Errorf("mining.min_difficulty must be <= max_difficulty")
	}

	if c.Mining.VardiffTargetTime <= 0 {
		return fmt.Errorf("mining.vardiff_target_time must be positive")
	}

	if c.Payouts.Threshold == 0 {
		return fmt.Errorf("payouts.threshold must be > 0")
	}

	return nil
}

// IsCombinedMode returns true if running master and slave together
func (c *Config) IsCombinedMode() bool {
	return c.Master.Enabled && c.Slave.Enabled
}

// IsMasterOnly returns true if running master only
func (c *Config) IsMasterOnly() bool {
	return c.Master.Enabled && !c.Slave.Enabled
}

// IsSlaveOnly returns true if running slave only
func (c *Config) IsSlaveOnly() bool {
	return !c.Master.Enabled && c.Slave.Enabled
}

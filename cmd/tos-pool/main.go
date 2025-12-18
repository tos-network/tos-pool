// TOS Pool - Mining pool for TOS Hash V3
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tos-network/tos-pool/internal/api"
	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/master"
	"github.com/tos-network/tos-pool/internal/newrelic"
	"github.com/tos-network/tos-pool/internal/policy"
	"github.com/tos-network/tos-pool/internal/profiling"
	"github.com/tos-network/tos-pool/internal/rpc"
	"github.com/tos-network/tos-pool/internal/slave"
	"github.com/tos-network/tos-pool/internal/storage"
	"github.com/tos-network/tos-pool/internal/util"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
)

func main() {
	// Command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	mode := flag.String("mode", "combined", "Run mode: combined, master, slave")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("TOS Pool v%s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := util.InitLogger(cfg.Log.Level, cfg.Log.Format, cfg.Log.File); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	util.Infof("TOS Pool v%s starting in %s mode", version, *mode)

	// Apply mode overrides
	switch *mode {
	case "master":
		cfg.Master.Enabled = true
		cfg.Slave.Enabled = false
	case "slave":
		cfg.Master.Enabled = false
		cfg.Slave.Enabled = true
	case "combined":
		cfg.Master.Enabled = true
		cfg.Slave.Enabled = true
	default:
		util.Fatalf("Invalid mode: %s", *mode)
	}

	// Connect to Redis
	redis, err := storage.NewRedisClient(cfg.Redis.URL, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		util.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redis.Close()

	// Create upstream manager with multi-node failover support
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upstreamMgr := rpc.NewUpstreamManager(ctx, &cfg.Node)
	// Set pool's fee address as miner address for get_block_template
	upstreamMgr.SetMinerAddress(cfg.Pool.FeeAddress)
	upstreamMgr.Start()

	var masterCoord *master.Master
	var stratum *slave.StratumServer
	var wsServer *slave.WebSocketServer
	var xatumServer *slave.XatumServer
	var apiServer *api.Server
	var policyServer *policy.PolicyServer
	var pprofServer *profiling.Server
	var nrAgent *newrelic.Agent

	// Initialize policy server for security
	policyConfig := policy.DefaultConfig()
	// Apply security settings from config file
	if cfg.Security.MaxConnectionsPerIP > 0 {
		policyConfig.ConnectionLimit = int32(cfg.Security.MaxConnectionsPerIP)
	}
	if cfg.Security.BanThreshold > 0 {
		policyConfig.CheckThreshold = int32(cfg.Security.BanThreshold)
	}
	if cfg.Security.BanDuration > 0 {
		policyConfig.BanTimeout = cfg.Security.BanDuration
	}
	if cfg.Security.RateLimitShares > 0 {
		policyConfig.MaxScore = int32(cfg.Security.RateLimitShares)
	}
	policyServer = policy.NewPolicyServer(policyConfig, redis)
	policyServer.Start()

	// Start pprof profiling server if enabled
	if cfg.Profiling.Enabled {
		pprofServer = profiling.NewServer(&cfg.Profiling)
		if err := pprofServer.Start(); err != nil {
			util.Errorf("Failed to start pprof server: %v", err)
		}
	}

	// Initialize New Relic APM if enabled
	if cfg.NewRelic.Enabled {
		nrAgent = newrelic.NewAgent(&cfg.NewRelic)
		if err := nrAgent.Start(); err != nil {
			util.Errorf("Failed to start New Relic agent: %v", err)
		}
	}

	// Start master if enabled
	if cfg.Master.Enabled {
		masterCoord = master.NewMaster(cfg, redis, upstreamMgr)
		if err := masterCoord.Start(); err != nil {
			util.Fatalf("Failed to start master: %v", err)
		}

		// Start API server
		if cfg.API.Enabled {
			apiServer = api.NewServer(cfg, redis)

			// Wire up upstream state callback for monitoring
			apiServer.SetUpstreamStateFunc(func() []api.UpstreamStatus {
				states := upstreamMgr.GetUpstreamStates()
				result := make([]api.UpstreamStatus, len(states))
				for i, s := range states {
					result[i] = api.UpstreamStatus{
						Name:         s.Name,
						URL:          s.URL,
						Healthy:      s.Healthy,
						ResponseTime: float64(s.ResponseTime.Milliseconds()),
						Height:       s.Height,
						Weight:       s.Weight,
						FailCount:    s.FailCount,
						SuccessCount: s.SuccessCount,
					}
				}
				return result
			})

			if err := apiServer.Start(); err != nil {
				util.Fatalf("Failed to start API server: %v", err)
			}
		}
	}

	// Start slave (stratum server) if enabled
	if cfg.Slave.Enabled {
		// Share callback for all mining protocols
		shareCallback := func(share *slave.Share) {
			if masterCoord != nil {
				submission := &master.ShareSubmission{
					Address:        share.Address,
					Worker:         share.Worker,
					JobID:          share.JobID,
					Nonce:          share.Nonce,
					Difficulty:     share.Difficulty,
					Height:         share.Height,
					TrustScore:     share.TrustScore,
					SkipValidation: share.SkipValidation,
				}
				masterCoord.SubmitShare(submission)
			}
			// Record share in New Relic
			if nrAgent != nil {
				nrAgent.RecordShareSubmission(share.Address, share.Worker, share.Difficulty, true)
			}
		}

		// Start Stratum server
		stratum = slave.NewStratumServer(cfg, policyServer)
		stratum.SetShareCallback(shareCallback)
		if err := stratum.Start(); err != nil {
			util.Fatalf("Failed to start stratum server: %v", err)
		}

		// Start WebSocket GetWork server if enabled
		if cfg.Slave.WebSocketEnabled {
			wsServer = slave.NewWebSocketServer(cfg, policyServer)
			wsServer.SetShareCallback(shareCallback)
			if err := wsServer.Start(); err != nil {
				util.Errorf("Failed to start WebSocket server: %v", err)
			}
		}

		// Start Xatum server if enabled
		if cfg.Slave.XatumEnabled {
			xatumServer = slave.NewXatumServer(cfg, policyServer)
			xatumServer.SetShareCallback(shareCallback)
			if err := xatumServer.Start(); err != nil {
				util.Errorf("Failed to start Xatum server: %v", err)
			}
		}

		// Broadcast jobs from master to all mining servers
		if masterCoord != nil {
			go func() {
				for {
					job := masterCoord.GetCurrentJob()
					if job != nil {
						stratumJob := &slave.Job{
							ID:         job.ID,
							Height:     job.Height,
							HeaderHash: util.BytesToHexNoPre(job.HeaderHash), // No 0x prefix for Stratum
							Target:     util.BytesToHexNoPre(job.Target),     // No 0x prefix for Stratum
							Difficulty: job.Difficulty,
							Timestamp:  job.Timestamp,
							CleanJobs:  true,
						}
						// Broadcast to Stratum
						stratum.BroadcastJob(stratumJob)
						// Broadcast to WebSocket
						if wsServer != nil {
							wsServer.BroadcastJob(stratumJob)
						}
						// Broadcast to Xatum
						if xatumServer != nil {
							xatumServer.BroadcastJob(stratumJob)
						}
					}
					// Wait for next job refresh
					<-masterCoord.GetJobUpdateChan()
				}
			}()
		}
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	util.Info("Pool started successfully. Press Ctrl+C to stop.")

	<-sigChan
	util.Info("Shutting down...")

	// Graceful shutdown
	if xatumServer != nil {
		xatumServer.Stop()
	}
	if wsServer != nil {
		wsServer.Stop()
	}
	if stratum != nil {
		stratum.Stop()
	}
	if apiServer != nil {
		apiServer.Stop()
	}
	if masterCoord != nil {
		masterCoord.Stop()
	}
	if policyServer != nil {
		policyServer.Stop()
	}
	if pprofServer != nil {
		pprofServer.Stop()
	}
	if nrAgent != nil {
		nrAgent.Stop()
	}
	upstreamMgr.Stop()

	util.Info("Pool stopped")
}

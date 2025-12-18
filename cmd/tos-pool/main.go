// TOS Pool - Mining pool for TOS Hash V3
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tos-network/tos-pool/internal/api"
	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/master"
	"github.com/tos-network/tos-pool/internal/policy"
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

	// Connect to TOS node
	node := rpc.NewTOSClient(cfg.Node.URL, cfg.Node.Timeout)

	var masterCoord *master.Master
	var stratum *slave.StratumServer
	var apiServer *api.Server
	var policyServer *policy.PolicyServer

	// Initialize policy server for security
	policyConfig := policy.DefaultConfig()
	// TODO: Load policy config from config file
	policyServer = policy.NewPolicyServer(policyConfig, redis)
	policyServer.Start()

	// Start master if enabled
	if cfg.Master.Enabled {
		masterCoord = master.NewMaster(cfg, redis, node)
		if err := masterCoord.Start(); err != nil {
			util.Fatalf("Failed to start master: %v", err)
		}

		// Start API server
		if cfg.API.Enabled {
			apiServer = api.NewServer(cfg, redis)
			if err := apiServer.Start(); err != nil {
				util.Fatalf("Failed to start API server: %v", err)
			}
		}
	}

	// Start slave (stratum server) if enabled
	if cfg.Slave.Enabled {
		stratum = slave.NewStratumServer(cfg, policyServer)

		// Set callbacks
		stratum.SetShareCallback(func(share *slave.Share) {
			if masterCoord != nil {
				submission := &master.ShareSubmission{
					Address:    share.Address,
					Worker:     share.Worker,
					JobID:      share.JobID,
					Nonce:      share.Nonce,
					Difficulty: share.Difficulty,
					Height:     share.Height,
				}
				masterCoord.SubmitShare(submission)
			}
		})

		if err := stratum.Start(); err != nil {
			util.Fatalf("Failed to start stratum server: %v", err)
		}

		// Broadcast jobs from master to stratum
		if masterCoord != nil {
			go func() {
				for {
					job := masterCoord.GetCurrentJob()
					if job != nil {
						stratumJob := &slave.Job{
							ID:         job.ID,
							Height:     job.Height,
							HeaderHash: util.BytesToHex(job.HeaderHash),
							Target:     util.BytesToHex(job.Target),
							Difficulty: job.Difficulty,
							Timestamp:  job.Timestamp,
							CleanJobs:  true,
						}
						stratum.BroadcastJob(stratumJob)
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

	util.Info("Pool stopped")
}

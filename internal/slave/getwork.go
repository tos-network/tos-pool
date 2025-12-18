// Package slave provides the getwork HTTP protocol for solo miners.
package slave

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/master"
	"github.com/tos-network/tos-pool/internal/toshash"
	"github.com/tos-network/tos-pool/internal/util"
)

// GetworkServer handles HTTP getwork requests for solo miners
type GetworkServer struct {
	cfg    *config.Config
	master *master.Master
	server *http.Server

	// Current work
	currentWork *GetworkJob
	workMu      sync.RWMutex

	// Session tracking
	sessions   map[string]*GetworkSession
	sessionsMu sync.RWMutex
}

// GetworkSession tracks a getwork miner session
type GetworkSession struct {
	Address    string
	Worker     string
	LastSeen   time.Time
	Difficulty uint64
	Shares     uint64
}

// GetworkJob represents work data in getwork format
type GetworkJob struct {
	JobID      string `json:"job_id"`
	HeaderHash string `json:"header_hash"`
	Target     string `json:"target"`
	Height     uint64 `json:"height"`
	Difficulty uint64 `json:"difficulty"`
	Timestamp  int64  `json:"timestamp"`
}

// GetworkRequest represents an incoming getwork JSON-RPC request
type GetworkRequest struct {
	ID      interface{}   `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	JSONRPC string        `json:"jsonrpc"`
}

// GetworkResponse represents a getwork JSON-RPC response
type GetworkResponse struct {
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	JSONRPC string      `json:"jsonrpc"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewGetworkServer creates a new getwork server
func NewGetworkServer(cfg *config.Config, m *master.Master) *GetworkServer {
	return &GetworkServer{
		cfg:      cfg,
		master:   m,
		sessions: make(map[string]*GetworkSession),
	}
}

// Start begins the getwork HTTP server
func (g *GetworkServer) Start(bind string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", g.handleRequest)
	mux.HandleFunc("/getwork", g.handleRequest)

	g.server = &http.Server{
		Addr:         bind,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start work updater
	go g.workUpdateLoop()

	util.Infof("Getwork server listening on %s", bind)

	go func() {
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			util.Errorf("Getwork server error: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the getwork server
func (g *GetworkServer) Stop() error {
	if g.server != nil {
		return g.server.Close()
	}
	return nil
}

// workUpdateLoop updates current work when new jobs arrive
func (g *GetworkServer) workUpdateLoop() {
	jobChan := g.master.GetJobUpdateChan()

	for range jobChan {
		job := g.master.GetCurrentJob()
		if job == nil {
			continue
		}

		g.workMu.Lock()
		g.currentWork = &GetworkJob{
			JobID:      job.ID,
			HeaderHash: util.BytesToHex(job.HeaderHash),
			Target:     util.BytesToHex(job.Target),
			Height:     job.Height,
			Difficulty: job.Difficulty,
			Timestamp:  time.Now().Unix(),
		}
		g.workMu.Unlock()

		util.Debugf("Getwork: Updated work for height %d", job.Height)
	}
}

// handleRequest processes incoming HTTP requests
func (g *GetworkServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// GET request - return current work directly (simplified getwork)
		if r.Method == http.MethodGet {
			g.handleGetWork(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendError(w, nil, -32700, "Parse error")
		return
	}

	switch req.Method {
	case "eth_getWork", "getwork", "mining.getwork":
		g.handleGetWorkRPC(w, &req)
	case "eth_submitWork", "submitwork", "mining.submit":
		g.handleSubmitWork(w, &req, r.RemoteAddr)
	case "eth_submitLogin", "mining.authorize":
		g.handleAuthorize(w, &req, r.RemoteAddr)
	default:
		g.sendError(w, req.ID, -32601, "Method not found")
	}
}

// handleGetWork returns current work via GET request
func (g *GetworkServer) handleGetWork(w http.ResponseWriter, r *http.Request) {
	g.workMu.RLock()
	work := g.currentWork
	g.workMu.RUnlock()

	if work == nil {
		http.Error(w, "No work available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(work)
}

// handleGetWorkRPC returns current work via JSON-RPC
func (g *GetworkServer) handleGetWorkRPC(w http.ResponseWriter, req *GetworkRequest) {
	g.workMu.RLock()
	work := g.currentWork
	g.workMu.RUnlock()

	if work == nil {
		g.sendError(w, req.ID, -1, "No work available")
		return
	}

	// Return work in eth_getWork format: [headerHash, seedHash, target]
	// For TOS, we use: [headerHash, jobID, target]
	result := []string{
		work.HeaderHash,
		work.JobID,
		work.Target,
	}

	g.sendResult(w, req.ID, result)
}

// handleAuthorize handles miner authorization
func (g *GetworkServer) handleAuthorize(w http.ResponseWriter, req *GetworkRequest, remoteAddr string) {
	if len(req.Params) < 1 {
		g.sendError(w, req.ID, -1, "Missing parameters")
		return
	}

	// Parse address.worker format
	login, _ := req.Params[0].(string)
	address, worker := parseLogin(login)

	if !util.ValidateAddress(address) {
		g.sendError(w, req.ID, -1, "Invalid address")
		return
	}

	// Create session
	g.sessionsMu.Lock()
	g.sessions[remoteAddr] = &GetworkSession{
		Address:    address,
		Worker:     worker,
		LastSeen:   time.Now(),
		Difficulty: g.cfg.Mining.InitialDifficulty,
	}
	g.sessionsMu.Unlock()

	util.Infof("Getwork: Authorized %s.%s from %s", address[:16], worker, remoteAddr)

	g.sendResult(w, req.ID, true)
}

// handleSubmitWork handles work submission
func (g *GetworkServer) handleSubmitWork(w http.ResponseWriter, req *GetworkRequest, remoteAddr string) {
	if len(req.Params) < 3 {
		g.sendError(w, req.ID, -1, "Missing parameters")
		return
	}

	// Parameters: [nonce, headerHash, mixDigest] or [nonce, jobID, hash]
	nonce, _ := req.Params[0].(string)
	headerHash, _ := req.Params[1].(string)
	// mixDigest is optional for TOS

	// Get session
	g.sessionsMu.RLock()
	session := g.sessions[remoteAddr]
	g.sessionsMu.RUnlock()

	if session == nil {
		// Allow anonymous submissions with address in params
		if len(req.Params) >= 4 {
			address, _ := req.Params[3].(string)
			if util.ValidateAddress(address) {
				session = &GetworkSession{
					Address:    address,
					Worker:     "getwork",
					Difficulty: g.cfg.Mining.InitialDifficulty,
				}
			}
		}

		if session == nil {
			g.sendError(w, req.ID, -1, "Not authorized")
			return
		}
	}

	session.LastSeen = time.Now()

	// Get current job
	job := g.master.GetCurrentJob()
	if job == nil {
		g.sendError(w, req.ID, -1, "No active job")
		return
	}

	// Verify header hash matches (for stale detection)
	jobHeaderHash := util.BytesToHex(job.HeaderHash)
	if headerHash != jobHeaderHash && headerHash != job.ID {
		g.sendError(w, req.ID, -1, "Stale work")
		return
	}

	// Parse nonce
	nonceBytes, err := util.HexToBytes(nonce)
	if err != nil {
		g.sendError(w, req.ID, -1, "Invalid nonce")
		return
	}

	// Build header with nonce
	header := make([]byte, toshash.InputSize)
	copy(header, job.HeaderHash)
	copy(header[72:80], nonceBytes)

	// Compute hash
	hash := toshash.Hash(header)
	if hash == nil {
		g.sendError(w, req.ID, -1, "Hash computation failed")
		return
	}

	// Check difficulty
	actualDiff := toshash.HashToDifficulty(hash)
	if actualDiff < session.Difficulty {
		g.sendError(w, req.ID, -1, "Low difficulty")
		return
	}

	// Submit share through master
	share := &master.ShareSubmission{
		Address:    session.Address,
		Worker:     session.Worker,
		JobID:      job.ID,
		Nonce:      nonce,
		Difficulty: session.Difficulty,
		Height:     job.Height,
	}

	result := g.master.SubmitShare(share)
	if !result.Valid {
		g.sendError(w, req.ID, -1, result.Message)
		return
	}

	session.Shares++

	if result.Block {
		util.Infof("Getwork: Block found by %s.%s!", session.Address[:16], session.Worker)
	}

	g.sendResult(w, req.ID, true)
}

// sendResult sends a successful JSON-RPC response
func (g *GetworkServer) sendResult(w http.ResponseWriter, id interface{}, result interface{}) {
	resp := GetworkResponse{
		ID:      id,
		Result:  result,
		JSONRPC: "2.0",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// sendError sends an error JSON-RPC response
func (g *GetworkServer) sendError(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := GetworkResponse{
		ID: id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
		JSONRPC: "2.0",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// parseLogin parses address.worker format
func parseLogin(login string) (address, worker string) {
	for i := len(login) - 1; i >= 0; i-- {
		if login[i] == '.' {
			return login[:i], login[i+1:]
		}
	}
	return login, "default"
}

// GetActiveSessions returns the number of active getwork sessions
func (g *GetworkServer) GetActiveSessions() int {
	g.sessionsMu.RLock()
	defer g.sessionsMu.RUnlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	count := 0
	for _, session := range g.sessions {
		if session.LastSeen.After(cutoff) {
			count++
		}
	}
	return count
}

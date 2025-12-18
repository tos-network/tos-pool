// Package slave implements the mining server (Stratum protocol).
package slave

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/policy"
	"github.com/tos-network/tos-pool/internal/util"
)

// Security constants
const (
	MaxRequestSize   = 1024            // Maximum request size in bytes
	MaxRequestBuffer = MaxRequestSize + 64 // Buffer size with some headroom
)

// StratumServer handles miner connections
type StratumServer struct {
	cfg       *config.Config
	listener  net.Listener
	tlsListener net.Listener

	// Security policy
	policy *policy.PolicyServer

	sessions   sync.Map // sessionID -> *Session
	sessionSeq uint64

	// Job management
	currentJob   atomic.Value // *Job
	jobSeq       uint64
	extraNonceSeq uint32

	// Callbacks
	onShare  func(*Share)
	onBlock  func(*Share)

	// Control
	quit chan struct{}
	wg   sync.WaitGroup
}

// Job represents mining work
type Job struct {
	ID           string
	Height       uint64
	HeaderHash   string
	ParentHash   string
	Target       string
	Difficulty   uint64
	Timestamp    uint64
	CleanJobs    bool
	CreatedAt    time.Time
}

// Share represents a submitted share
type Share struct {
	SessionID  uint64
	Address    string
	Worker     string
	JobID      string
	Nonce      string
	Hash       string
	Difficulty uint64
	Height     uint64
	Timestamp  int64
	IsBlock    bool
}

// Session represents a miner connection
type Session struct {
	ID         uint64
	Conn       net.Conn
	Reader     *bufio.Reader
	Address    string
	Worker     string
	Authorized bool

	// Difficulty
	Difficulty     uint64
	VardiffStats   *VardiffStats

	// Extra nonce
	ExtraNonce1    string
	ExtraNonce2Size int

	// Stats
	ValidShares   uint64
	InvalidShares uint64
	StaleShares   uint64
	LastShare     time.Time

	// Trust score
	TrustScore int

	// Connection info
	RemoteAddr string
	ConnectedAt time.Time

	// Control
	mu   sync.Mutex
	quit chan struct{}
}

// VardiffStats tracks share submission rate for difficulty adjustment
type VardiffStats struct {
	LastRetarget time.Time
	SharesSince  int
}

// StratumRequest is a JSON-RPC request from miner
type StratumRequest struct {
	ID     interface{}   `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// StratumResponse is a JSON-RPC response to miner
type StratumResponse struct {
	ID     interface{} `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  interface{} `json:"error,omitempty"`
}

// StratumNotify is a server notification to miner
type StratumNotify struct {
	ID     interface{}   `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// NewStratumServer creates a new Stratum server
func NewStratumServer(cfg *config.Config, policyServer *policy.PolicyServer) *StratumServer {
	s := &StratumServer{
		cfg:    cfg,
		policy: policyServer,
		quit:   make(chan struct{}),
	}
	return s
}

// SetShareCallback sets the share submission callback
func (s *StratumServer) SetShareCallback(fn func(*Share)) {
	s.onShare = fn
}

// SetBlockCallback sets the block found callback
func (s *StratumServer) SetBlockCallback(fn func(*Share)) {
	s.onBlock = fn
}

// Start begins listening for connections
func (s *StratumServer) Start() error {
	// Start TCP listener
	listener, err := net.Listen("tcp", s.cfg.Slave.StratumBind)
	if err != nil {
		return fmt.Errorf("failed to bind stratum server: %w", err)
	}
	s.listener = listener
	util.Infof("Stratum server listening on %s", s.cfg.Slave.StratumBind)

	// Start TLS listener if configured
	if s.cfg.Slave.TLSCert != "" && s.cfg.Slave.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(s.cfg.Slave.TLSCert, s.cfg.Slave.TLSKey)
		if err != nil {
			util.Warnf("Failed to load TLS cert/key: %v", err)
		} else {
			tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
			tlsListener, err := tls.Listen("tcp", s.cfg.Slave.StratumTLSBind, tlsConfig)
			if err != nil {
				util.Warnf("Failed to bind TLS stratum server: %v", err)
			} else {
				s.tlsListener = tlsListener
				util.Infof("Stratum TLS server listening on %s", s.cfg.Slave.StratumTLSBind)
			}
		}
	}

	// Accept connections
	s.wg.Add(1)
	go s.acceptLoop(s.listener)

	if s.tlsListener != nil {
		s.wg.Add(1)
		go s.acceptLoop(s.tlsListener)
	}

	return nil
}

// Stop shuts down the server
func (s *StratumServer) Stop() {
	close(s.quit)

	if s.listener != nil {
		s.listener.Close()
	}
	if s.tlsListener != nil {
		s.tlsListener.Close()
	}

	// Close all sessions
	s.sessions.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		session.Conn.Close()
		return true
	})

	s.wg.Wait()
	util.Info("Stratum server stopped")
}

// acceptLoop handles incoming connections
func (s *StratumServer) acceptLoop(listener net.Listener) {
	defer s.wg.Done()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				util.Warnf("Accept error: %v", err)
				continue
			}
		}

		// Extract IP from remote address
		ip := extractIP(conn.RemoteAddr().String())

		// Apply policy checks
		if s.policy != nil {
			// Check if IP is banned
			if s.policy.IsBanned(ip) {
				util.Debugf("Rejected banned IP: %s", ip)
				conn.Close()
				continue
			}

			// Check connection rate limit
			if !s.policy.ApplyConnectionLimit(ip) {
				util.Debugf("Connection limit exceeded for IP: %s", ip)
				conn.Close()
				continue
			}
		}

		// Create session
		session := s.createSession(conn)
		s.sessions.Store(session.ID, session)

		// Handle connection
		s.wg.Add(1)
		go s.handleSession(session)
	}
}

// createSession creates a new miner session
func (s *StratumServer) createSession(conn net.Conn) *Session {
	id := atomic.AddUint64(&s.sessionSeq, 1)
	extraNonce := atomic.AddUint32(&s.extraNonceSeq, 1)

	session := &Session{
		ID:             id,
		Conn:           conn,
		Reader:         bufio.NewReader(conn),
		ExtraNonce1:    fmt.Sprintf("%08x", extraNonce),
		ExtraNonce2Size: 4,
		Difficulty:     s.cfg.Mining.InitialDifficulty,
		VardiffStats: &VardiffStats{
			LastRetarget: time.Now(),
		},
		RemoteAddr:  conn.RemoteAddr().String(),
		ConnectedAt: time.Now(),
		quit:        make(chan struct{}),
	}

	return session
}

// handleSession processes messages from a miner
func (s *StratumServer) handleSession(session *Session) {
	defer s.wg.Done()
	defer func() {
		session.Conn.Close()
		s.sessions.Delete(session.ID)
		close(session.quit)
		util.Debugf("Session %d disconnected: %s", session.ID, session.RemoteAddr)
	}()

	util.Debugf("New connection from %s (session %d)", session.RemoteAddr, session.ID)

	ip := extractIP(session.RemoteAddr)

	// Set initial timeout
	session.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Use limited reader to prevent memory exhaustion
	limitedReader := bufio.NewReaderSize(session.Conn, MaxRequestBuffer)

	for {
		select {
		case <-s.quit:
			return
		default:
		}

		// Read line with flood detection
		line, isPrefix, err := limitedReader.ReadLine()
		if err != nil {
			return
		}

		// Socket flood detection: line too long (buffer overflow attempt)
		if isPrefix {
			util.Warnf("Session %d (%s): request too large (flood detected)", session.ID, ip)
			if s.policy != nil {
				s.policy.BanIP(ip)
			}
			return
		}

		// Check max request size
		if len(line) > MaxRequestSize {
			util.Warnf("Session %d (%s): request exceeds max size (%d > %d)",
				session.ID, ip, len(line), MaxRequestSize)
			if s.policy != nil {
				if !s.policy.ApplyMalformedPolicy(ip) {
					return
				}
			}
			s.sendError(session, nil, -32600, "Request too large")
			continue
		}

		// Reset timeout on activity
		session.Conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

		// Parse request
		var req StratumRequest
		if err := json.Unmarshal(line, &req); err != nil {
			// Apply malformed request policy
			if s.policy != nil {
				if !s.policy.ApplyMalformedPolicy(ip) {
					util.Warnf("Session %d (%s): banned for malformed requests", session.ID, ip)
					return
				}
			}
			s.sendError(session, nil, -32700, "Parse error")
			continue
		}

		// Handle request
		s.handleRequest(session, &req)
	}
}

// handleRequest processes a stratum request
func (s *StratumServer) handleRequest(session *Session, req *StratumRequest) {
	switch req.Method {
	case "mining.subscribe":
		s.handleSubscribe(session, req)
	case "mining.authorize":
		s.handleAuthorize(session, req)
	case "mining.submit":
		s.handleSubmit(session, req)
	case "mining.extranonce.subscribe":
		s.sendResult(session, req.ID, true)
	default:
		s.sendError(session, req.ID, -32601, "Method not found")
	}
}

// handleSubscribe processes mining.subscribe
func (s *StratumServer) handleSubscribe(session *Session, req *StratumRequest) {
	// Extract miner software info if provided
	if len(req.Params) > 0 {
		if minerSW, ok := req.Params[0].(string); ok {
			util.Debugf("Session %d: miner software: %s", session.ID, minerSW)
		}
	}

	// Response: [[["mining.notify", "subscription_id"]], extranonce1, extranonce2_size]
	result := []interface{}{
		[][]string{
			{"mining.notify", fmt.Sprintf("%d", session.ID)},
			{"mining.set_difficulty", fmt.Sprintf("%d", session.ID)},
		},
		session.ExtraNonce1,
		session.ExtraNonce2Size,
	}

	s.sendResult(session, req.ID, result)

	// Send initial difficulty
	s.sendDifficulty(session, session.Difficulty)

	// Send current job if available
	if job := s.getCurrentJob(); job != nil {
		s.sendJob(session, job)
	}
}

// handleAuthorize processes mining.authorize
func (s *StratumServer) handleAuthorize(session *Session, req *StratumRequest) {
	if len(req.Params) < 1 {
		s.sendError(session, req.ID, -1, "Invalid params")
		return
	}

	username, ok := req.Params[0].(string)
	if !ok {
		s.sendError(session, req.ID, -1, "Invalid username")
		return
	}

	// Parse address.worker format
	address, worker := parseWorkerID(username)

	// Validate address
	if !util.ValidateAddress(address) {
		s.sendError(session, req.ID, -1, "Invalid TOS address")
		return
	}

	// Check blacklist policy
	ip := extractIP(session.RemoteAddr)
	if s.policy != nil {
		if !s.policy.ApplyLoginPolicy(address, ip) {
			util.Warnf("Session %d (%s): blacklisted address %s", session.ID, ip, address)
			s.sendError(session, req.ID, -1, "Address blacklisted")
			return
		}
	}

	session.mu.Lock()
	session.Address = address
	session.Worker = worker
	session.Authorized = true
	session.mu.Unlock()

	util.Infof("Session %d authorized: %s.%s", session.ID, address[:16], worker)

	s.sendResult(session, req.ID, true)
}

// handleSubmit processes mining.submit
func (s *StratumServer) handleSubmit(session *Session, req *StratumRequest) {
	if !session.Authorized {
		s.sendError(session, req.ID, 24, "Unauthorized")
		return
	}

	ip := extractIP(session.RemoteAddr)

	// Params format 1 (tosminer): [worker, job_id, extranonce2, nonce]
	// Params format 2 (standard): [worker, job_id, extranonce2, ntime, nonce]
	if len(req.Params) < 4 {
		s.sendError(session, req.ID, -1, "Invalid params")
		atomic.AddUint64(&session.InvalidShares, 1)
		// Apply share policy for invalid share
		if s.policy != nil {
			if !s.policy.ApplySharePolicy(ip, false) {
				util.Warnf("Session %d (%s): banned for invalid share ratio", session.ID, ip)
				session.Conn.Close()
			}
		}
		return
	}

	jobID, _ := req.Params[1].(string)
	var nonce string
	if len(req.Params) >= 5 {
		// Standard 5-param format: nonce is at index 4
		nonce, _ = req.Params[4].(string)
	} else {
		// tosminer 4-param format: nonce is at index 3
		nonce, _ = req.Params[3].(string)
	}

	// Create share
	share := &Share{
		SessionID:  session.ID,
		Address:    session.Address,
		Worker:     session.Worker,
		JobID:      jobID,
		Nonce:      nonce,
		Difficulty: session.Difficulty,
		Timestamp:  time.Now().Unix(),
	}

	// Validate share (simplified - actual validation done in handler)
	job := s.getCurrentJob()
	if job == nil || job.ID != jobID {
		s.sendError(session, req.ID, 21, "Job not found")
		atomic.AddUint64(&session.StaleShares, 1)
		// Stale shares count as invalid for policy
		if s.policy != nil {
			if !s.policy.ApplySharePolicy(ip, false) {
				util.Warnf("Session %d (%s): banned for invalid share ratio", session.ID, ip)
				session.Conn.Close()
			}
		}
		return
	}

	share.Height = job.Height

	// Update vardiff stats
	session.VardiffStats.SharesSince++
	session.LastShare = time.Now()

	// Check if difficulty adjustment needed
	s.checkVardiff(session)

	// Accept share (actual validation done by callback)
	atomic.AddUint64(&session.ValidShares, 1)
	session.TrustScore++

	// Apply share policy for valid share
	if s.policy != nil {
		s.policy.ApplySharePolicy(ip, true)
	}

	// Callback
	if s.onShare != nil {
		s.onShare(share)
	}

	s.sendResult(session, req.ID, true)
}

// ReportInvalidShare allows callbacks to report a share as invalid (for policy tracking)
func (s *StratumServer) ReportInvalidShare(sessionID uint64, ip string) {
	if s.policy != nil {
		if !s.policy.ApplySharePolicy(ip, false) {
			// Find and close the session
			if val, ok := s.sessions.Load(sessionID); ok {
				session := val.(*Session)
				session.Conn.Close()
			}
		}
	}
}

// checkVardiff checks if difficulty adjustment is needed
func (s *StratumServer) checkVardiff(session *Session) {
	elapsed := time.Since(session.VardiffStats.LastRetarget).Seconds()
	if elapsed < s.cfg.Mining.VardiffRetarget {
		return
	}

	// Calculate actual share rate
	shareRate := float64(session.VardiffStats.SharesSince) / elapsed
	targetRate := 1.0 / s.cfg.Mining.VardiffTargetTime

	// Calculate ratio
	ratio := shareRate / targetRate
	if ratio == 0 {
		ratio = 0.5 // Default to halving if no shares
	}

	// Apply variance limits
	variance := s.cfg.Mining.VardiffVariance / 100.0
	if ratio > 1+variance {
		ratio = 1 + variance
	} else if ratio < 1-variance {
		ratio = 1 - variance
	}

	// Calculate new difficulty
	newDiff := uint64(float64(session.Difficulty) * ratio)

	// Clamp to bounds
	if newDiff < s.cfg.Mining.MinDifficulty {
		newDiff = s.cfg.Mining.MinDifficulty
	}
	if newDiff > s.cfg.Mining.MaxDifficulty {
		newDiff = s.cfg.Mining.MaxDifficulty
	}

	// Update if changed significantly
	if newDiff != session.Difficulty {
		session.Difficulty = newDiff
		s.sendDifficulty(session, newDiff)
		util.Debugf("Session %d difficulty adjusted to %d", session.ID, newDiff)
	}

	// Reset stats
	session.VardiffStats.LastRetarget = time.Now()
	session.VardiffStats.SharesSince = 0
}

// BroadcastJob sends a new job to all connected miners
func (s *StratumServer) BroadcastJob(job *Job) {
	s.currentJob.Store(job)
	atomic.AddUint64(&s.jobSeq, 1)

	s.sessions.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		if session.Authorized {
			s.sendJob(session, job)
		}
		return true
	})

	util.Debugf("Broadcasted job %s at height %d", job.ID, job.Height)
}

// getCurrentJob returns the current mining job
func (s *StratumServer) getCurrentJob() *Job {
	if job := s.currentJob.Load(); job != nil {
		return job.(*Job)
	}
	return nil
}

// sendJob sends a job notification to a miner
// Format: [job_id, header_hex, target, height, clean_jobs]
// This matches tosminer's expected format
func (s *StratumServer) sendJob(session *Session, job *Job) {
	notify := StratumNotify{
		ID:     nil,
		Method: "mining.notify",
		Params: []interface{}{
			job.ID,
			job.HeaderHash,
			job.Target,
			job.Height,
			job.CleanJobs,
		},
	}
	s.send(session, notify)
}

// sendDifficulty sends difficulty update to miner
func (s *StratumServer) sendDifficulty(session *Session, difficulty uint64) {
	notify := StratumNotify{
		ID:     nil,
		Method: "mining.set_difficulty",
		Params: []interface{}{difficulty},
	}
	s.send(session, notify)
}

// sendResult sends a success response
func (s *StratumServer) sendResult(session *Session, id interface{}, result interface{}) {
	resp := StratumResponse{
		ID:     id,
		Result: result,
	}
	s.send(session, resp)
}

// sendError sends an error response
func (s *StratumServer) sendError(session *Session, id interface{}, code int, message string) {
	resp := StratumResponse{
		ID:    id,
		Error: []interface{}{code, message, nil},
	}
	s.send(session, resp)
}

// send writes a message to the miner
func (s *StratumServer) send(session *Session, msg interface{}) {
	session.mu.Lock()
	defer session.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	session.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	session.Conn.Write(append(data, '\n'))
}

// parseWorkerID parses "address.worker" format
func parseWorkerID(username string) (address, worker string) {
	for i, c := range username {
		if c == '.' {
			return username[:i], username[i+1:]
		}
	}
	return username, "default"
}

// GetSessionCount returns number of connected sessions
func (s *StratumServer) GetSessionCount() int {
	count := 0
	s.sessions.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetAuthorizedCount returns number of authorized sessions
func (s *StratumServer) GetAuthorizedCount() int {
	count := 0
	s.sessions.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		if session.Authorized {
			count++
		}
		return true
	})
	return count
}

// GetPolicy returns the policy server for external access
func (s *StratumServer) GetPolicy() *policy.PolicyServer {
	return s.policy
}

// extractIP extracts the IP address from a remote address string (ip:port)
func extractIP(remoteAddr string) string {
	// Handle IPv6 addresses like [::1]:port
	if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
		ip := remoteAddr[:idx]
		// Remove brackets from IPv6
		ip = strings.TrimPrefix(ip, "[")
		ip = strings.TrimSuffix(ip, "]")
		return ip
	}
	return remoteAddr
}

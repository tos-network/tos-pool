// Xatum protocol server - TLS-secured JSON mining protocol.
// Xatum is a cleaner JSON-based alternative to Stratum with mandatory TLS encryption.
package slave

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/policy"
	"github.com/tos-network/tos-pool/internal/util"
)

// XatumServer handles Xatum protocol connections
type XatumServer struct {
	cfg      *config.Config
	policy   *policy.PolicyServer
	listener net.Listener

	sessions   sync.Map // sessionID -> *XatumSession
	sessionSeq uint64

	currentJob atomic.Value // *Job

	onShare func(*Share)

	quit chan struct{}
	wg   sync.WaitGroup
}

// XatumSession represents a Xatum client session
type XatumSession struct {
	ID          uint64
	Conn        net.Conn
	Reader      *bufio.Reader
	Address     string
	Worker      string
	Authorized  bool
	Difficulty  uint64
	RemoteAddr  string
	ConnectedAt time.Time

	mu   sync.Mutex
	quit chan struct{}
}

// XatumRequest is a Xatum protocol request
type XatumRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// XatumResponse is a Xatum protocol response
type XatumResponse struct {
	ID     string      `json:"id,omitempty"`
	Method string      `json:"method,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error  *XatumError `json:"error,omitempty"`
}

// XatumError represents a Xatum error
type XatumError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Xatum method constants
const (
	XatumMethodHandshake = "handshake"
	XatumMethodAuthorize = "authorize"
	XatumMethodJob       = "job"
	XatumMethodSubmit    = "submit"
	XatumMethodPing      = "ping"
	XatumMethodPong      = "pong"
)

// HandshakeParams represents handshake parameters
type HandshakeParams struct {
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
}

// HandshakeResult represents handshake response
type HandshakeResult struct {
	Version    string `json:"version"`
	Protocol   string `json:"protocol"`
	SessionID  string `json:"session_id"`
	Difficulty uint64 `json:"difficulty"`
}

// AuthorizeParams represents authorize parameters
type AuthorizeParams struct {
	Address string `json:"address"`
	Worker  string `json:"worker"`
}

// JobParams represents job notification
type JobParams struct {
	ID         string `json:"id"`
	HeaderHash string `json:"header_hash"`
	Target     string `json:"target"`
	Height     uint64 `json:"height"`
	Difficulty uint64 `json:"difficulty"`
	Clean      bool   `json:"clean"`
}

// SubmitParams represents submit parameters
type SubmitParams struct {
	JobID string `json:"job_id"`
	Nonce string `json:"nonce"`
}

// NewXatumServer creates a new Xatum server
func NewXatumServer(cfg *config.Config, policyServer *policy.PolicyServer) *XatumServer {
	return &XatumServer{
		cfg:    cfg,
		policy: policyServer,
		quit:   make(chan struct{}),
	}
}

// SetShareCallback sets the share submission callback
func (s *XatumServer) SetShareCallback(fn func(*Share)) {
	s.onShare = fn
}

// Start begins the Xatum server
func (s *XatumServer) Start() error {
	if !s.cfg.Slave.XatumEnabled {
		return nil
	}

	// Xatum requires TLS
	if s.cfg.Slave.TLSCert == "" || s.cfg.Slave.TLSKey == "" {
		util.Warn("Xatum requires TLS certificate and key")
		return nil
	}

	cert, err := tls.LoadX509KeyPair(s.cfg.Slave.TLSCert, s.cfg.Slave.TLSKey)
	if err != nil {
		util.Errorf("Failed to load TLS cert for Xatum: %v", err)
		return nil
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener, err := tls.Listen("tcp", s.cfg.Slave.XatumBind, tlsConfig)
	if err != nil {
		return err
	}

	s.listener = listener
	util.Infof("Xatum server listening on %s (TLS)", s.cfg.Slave.XatumBind)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop shuts down the server
func (s *XatumServer) Stop() {
	close(s.quit)

	if s.listener != nil {
		s.listener.Close()
	}

	s.sessions.Range(func(key, value interface{}) bool {
		session := value.(*XatumSession)
		session.Conn.Close()
		return true
	})

	s.wg.Wait()
	util.Info("Xatum server stopped")
}

// acceptLoop handles incoming connections
func (s *XatumServer) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				util.Warnf("Xatum accept error: %v", err)
				continue
			}
		}

		ip := extractIP(conn.RemoteAddr().String())

		if s.policy != nil {
			if s.policy.IsBanned(ip) {
				conn.Close()
				continue
			}
			if !s.policy.ApplyConnectionLimit(ip) {
				conn.Close()
				continue
			}
		}

		session := s.createSession(conn)
		s.sessions.Store(session.ID, session)

		s.wg.Add(1)
		go s.handleSession(session)
	}
}

// createSession creates a new Xatum session
func (s *XatumServer) createSession(conn net.Conn) *XatumSession {
	id := atomic.AddUint64(&s.sessionSeq, 1)

	return &XatumSession{
		ID:          id,
		Conn:        conn,
		Reader:      bufio.NewReader(conn),
		Difficulty:  s.cfg.Mining.InitialDifficulty,
		RemoteAddr:  conn.RemoteAddr().String(),
		ConnectedAt: time.Now(),
		quit:        make(chan struct{}),
	}
}

// handleSession processes messages from a session
func (s *XatumServer) handleSession(session *XatumSession) {
	defer s.wg.Done()
	defer func() {
		session.Conn.Close()
		s.sessions.Delete(session.ID)
		close(session.quit)
		util.Debugf("Xatum session %d disconnected", session.ID)
	}()

	util.Debugf("Xatum connection from %s (session %d)", session.RemoteAddr, session.ID)

	session.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	for {
		select {
		case <-s.quit:
			return
		default:
		}

		line, err := session.Reader.ReadBytes('\n')
		if err != nil {
			return
		}

		session.Conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

		var req XatumRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(session, "", -32700, "Parse error")
			continue
		}

		s.handleRequest(session, &req)
	}
}

// handleRequest processes a Xatum request
func (s *XatumServer) handleRequest(session *XatumSession, req *XatumRequest) {
	switch req.Method {
	case XatumMethodHandshake:
		s.handleHandshake(session, req)
	case XatumMethodAuthorize:
		s.handleAuthorize(session, req)
	case XatumMethodSubmit:
		s.handleSubmit(session, req)
	case XatumMethodPing:
		s.handlePing(session, req)
	default:
		s.sendError(session, req.ID, -32601, "Method not found")
	}
}

// handleHandshake processes handshake
func (s *XatumServer) handleHandshake(session *XatumSession, req *XatumRequest) {
	var params HandshakeParams
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}

	util.Debugf("Xatum session %d handshake: version=%s", session.ID, params.Version)

	result := HandshakeResult{
		Version:    "1.0.0",
		Protocol:   "xatum/1.0",
		SessionID:  util.Int64ToHex(int64(session.ID)),
		Difficulty: session.Difficulty,
	}

	s.sendResult(session, req.ID, result)
}

// handleAuthorize processes authorization
func (s *XatumServer) handleAuthorize(session *XatumSession, req *XatumRequest) {
	var params AuthorizeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(session, req.ID, -1, "Invalid params")
		return
	}

	// Validate address
	if !util.ValidateAddress(params.Address) {
		s.sendError(session, req.ID, -1, "Invalid TOS address")
		return
	}

	// Check blacklist
	ip := extractIP(session.RemoteAddr)
	if s.policy != nil {
		if !s.policy.ApplyLoginPolicy(params.Address, ip) {
			s.sendError(session, req.ID, -1, "Address blacklisted")
			return
		}
	}

	session.mu.Lock()
	session.Address = params.Address
	session.Worker = params.Worker
	if session.Worker == "" {
		session.Worker = "default"
	}
	session.Authorized = true
	session.mu.Unlock()

	util.Infof("Xatum session %d authorized: %s.%s", session.ID, params.Address[:16], session.Worker)

	s.sendResult(session, req.ID, true)

	// Send current job
	if job := s.getCurrentJob(); job != nil {
		s.sendJob(session, job)
	}
}

// handleSubmit processes share submission
func (s *XatumServer) handleSubmit(session *XatumSession, req *XatumRequest) {
	if !session.Authorized {
		s.sendError(session, req.ID, 24, "Unauthorized")
		return
	}

	var params SubmitParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(session, req.ID, -1, "Invalid params")
		return
	}

	job := s.getCurrentJob()
	if job == nil || job.ID != params.JobID {
		s.sendError(session, req.ID, 21, "Job not found")
		return
	}

	share := &Share{
		SessionID:  session.ID,
		Address:    session.Address,
		Worker:     session.Worker,
		JobID:      params.JobID,
		Nonce:      params.Nonce,
		Difficulty: session.Difficulty,
		Height:     job.Height,
		Timestamp:  time.Now().Unix(),
	}

	if s.onShare != nil {
		s.onShare(share)
	}

	s.sendResult(session, req.ID, true)
}

// handlePing processes ping
func (s *XatumServer) handlePing(session *XatumSession, req *XatumRequest) {
	s.sendMethod(session, XatumMethodPong, nil)
}

// BroadcastJob sends job to all sessions
func (s *XatumServer) BroadcastJob(job *Job) {
	s.currentJob.Store(job)

	s.sessions.Range(func(key, value interface{}) bool {
		session := value.(*XatumSession)
		if session.Authorized {
			s.sendJob(session, job)
		}
		return true
	})
}

// getCurrentJob returns current job
func (s *XatumServer) getCurrentJob() *Job {
	if job := s.currentJob.Load(); job != nil {
		return job.(*Job)
	}
	return nil
}

// sendJob sends job notification
func (s *XatumServer) sendJob(session *XatumSession, job *Job) {
	params := JobParams{
		ID:         job.ID,
		HeaderHash: job.HeaderHash,
		Target:     job.Target,
		Height:     job.Height,
		Difficulty: session.Difficulty,
		Clean:      job.CleanJobs,
	}
	s.sendMethod(session, XatumMethodJob, params)
}

// sendResult sends a success response
func (s *XatumServer) sendResult(session *XatumSession, id string, result interface{}) {
	resp := XatumResponse{
		ID:     id,
		Result: result,
	}
	s.send(session, resp)
}

// sendError sends an error response
func (s *XatumServer) sendError(session *XatumSession, id string, code int, message string) {
	resp := XatumResponse{
		ID: id,
		Error: &XatumError{
			Code:    code,
			Message: message,
		},
	}
	s.send(session, resp)
}

// sendMethod sends a method notification
func (s *XatumServer) sendMethod(session *XatumSession, method string, params interface{}) {
	resp := XatumResponse{
		Method: method,
		Result: params,
	}
	s.send(session, resp)
}

// send writes a message to session
func (s *XatumServer) send(session *XatumSession, msg interface{}) {
	session.mu.Lock()
	defer session.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	session.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	session.Conn.Write(append(data, '\n'))
}

// GetSessionCount returns number of sessions
func (s *XatumServer) GetSessionCount() int {
	count := 0
	s.sessions.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

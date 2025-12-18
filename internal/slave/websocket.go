// WebSocket GetWork server for real-time work notifications.
package slave

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tos-network/tos-pool/internal/config"
	"github.com/tos-network/tos-pool/internal/policy"
	"github.com/tos-network/tos-pool/internal/util"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for mining
	},
}

// WebSocketServer handles WebSocket GetWork connections
type WebSocketServer struct {
	cfg      *config.Config
	policy   *policy.PolicyServer
	server   *http.Server
	clients  sync.Map // clientID -> *WSClient
	clientSeq uint64

	// Current job
	currentJob atomic.Value // *Job

	// Callbacks
	onShare func(*Share)

	quit chan struct{}
	wg   sync.WaitGroup
}

// WSClient represents a WebSocket client
type WSClient struct {
	ID         uint64
	Conn       *websocket.Conn
	Address    string
	Worker     string
	Authorized bool
	Difficulty uint64
	RemoteAddr string
	ConnectedAt time.Time

	writeMu sync.Mutex
	quit    chan struct{}
}

// WSRequest is a JSON-RPC request from client
type WSRequest struct {
	ID     interface{}   `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// WSResponse is a JSON-RPC response to client
type WSResponse struct {
	ID     interface{} `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  interface{} `json:"error,omitempty"`
}

// WSNotify is a server notification
type WSNotify struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// GetWorkResult represents the getWork response
type GetWorkResult struct {
	HeaderHash string `json:"headerHash"`
	Target     string `json:"target"`
	Height     uint64 `json:"height"`
	JobID      string `json:"jobId"`
	Difficulty uint64 `json:"difficulty"`
}

// NewWebSocketServer creates a new WebSocket GetWork server
func NewWebSocketServer(cfg *config.Config, policyServer *policy.PolicyServer) *WebSocketServer {
	return &WebSocketServer{
		cfg:    cfg,
		policy: policyServer,
		quit:   make(chan struct{}),
	}
}

// SetShareCallback sets the share submission callback
func (s *WebSocketServer) SetShareCallback(fn func(*Share)) {
	s.onShare = fn
}

// Start begins the WebSocket server
func (s *WebSocketServer) Start() error {
	if !s.cfg.Slave.WebSocketEnabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleConnection)
	mux.HandleFunc("/", s.handleConnection) // Also accept on root

	s.server = &http.Server{
		Addr:    s.cfg.Slave.WebSocketBind,
		Handler: mux,
	}

	util.Infof("WebSocket GetWork server listening on %s", s.cfg.Slave.WebSocketBind)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			util.Errorf("WebSocket server error: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the server
func (s *WebSocketServer) Stop() {
	close(s.quit)

	if s.server != nil {
		s.server.Close()
	}

	// Close all clients
	s.clients.Range(func(key, value interface{}) bool {
		client := value.(*WSClient)
		client.Conn.Close()
		return true
	})

	s.wg.Wait()
	util.Info("WebSocket server stopped")
}

// handleConnection handles new WebSocket connections
func (s *WebSocketServer) handleConnection(w http.ResponseWriter, r *http.Request) {
	// Extract IP
	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = forwarded
	}

	// Check policy
	if s.policy != nil {
		if s.policy.IsBanned(ip) {
			http.Error(w, "Banned", http.StatusForbidden)
			return
		}
		if !s.policy.ApplyConnectionLimit(ip) {
			http.Error(w, "Too many connections", http.StatusTooManyRequests)
			return
		}
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		util.Warnf("WebSocket upgrade error: %v", err)
		return
	}

	// Create client
	client := &WSClient{
		ID:          atomic.AddUint64(&s.clientSeq, 1),
		Conn:        conn,
		Difficulty:  s.cfg.Mining.InitialDifficulty,
		RemoteAddr:  ip,
		ConnectedAt: time.Now(),
		quit:        make(chan struct{}),
	}

	s.clients.Store(client.ID, client)
	util.Debugf("WebSocket client %d connected from %s", client.ID, ip)

	s.wg.Add(1)
	go s.handleClient(client)
}

// handleClient processes messages from a client
func (s *WebSocketServer) handleClient(client *WSClient) {
	defer s.wg.Done()
	defer func() {
		client.Conn.Close()
		s.clients.Delete(client.ID)
		close(client.quit)
		util.Debugf("WebSocket client %d disconnected", client.ID)
	}()

	for {
		select {
		case <-s.quit:
			return
		default:
		}

		// Read message
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			return
		}

		// Parse request
		var req WSRequest
		if err := json.Unmarshal(message, &req); err != nil {
			s.sendError(client, nil, -32700, "Parse error")
			continue
		}

		// Handle request
		s.handleRequest(client, &req)
	}
}

// handleRequest processes a WebSocket request
func (s *WebSocketServer) handleRequest(client *WSClient, req *WSRequest) {
	switch req.Method {
	case "mining.authorize", "authorize":
		s.handleAuthorize(client, req)
	case "mining.getwork", "getwork", "tos_getWork":
		s.handleGetWork(client, req)
	case "mining.submit", "submit", "tos_submitWork":
		s.handleSubmit(client, req)
	case "mining.subscribe", "subscribe":
		s.handleSubscribe(client, req)
	default:
		s.sendError(client, req.ID, -32601, "Method not found")
	}
}

// handleAuthorize processes authorization
func (s *WebSocketServer) handleAuthorize(client *WSClient, req *WSRequest) {
	if len(req.Params) < 1 {
		s.sendError(client, req.ID, -1, "Invalid params")
		return
	}

	username, ok := req.Params[0].(string)
	if !ok {
		s.sendError(client, req.ID, -1, "Invalid username")
		return
	}

	// Parse address.worker format
	address, worker := parseWorkerID(username)

	// Validate address
	if !util.ValidateAddress(address) {
		s.sendError(client, req.ID, -1, "Invalid TOS address")
		return
	}

	// Check blacklist
	if s.policy != nil {
		if !s.policy.ApplyLoginPolicy(address, client.RemoteAddr) {
			s.sendError(client, req.ID, -1, "Address blacklisted")
			return
		}
	}

	client.Address = address
	client.Worker = worker
	client.Authorized = true

	util.Infof("WebSocket client %d authorized: %s.%s", client.ID, address[:16], worker)
	s.sendResult(client, req.ID, true)

	// Send current job
	if job := s.getCurrentJob(); job != nil {
		s.sendJob(client, job)
	}
}

// handleSubscribe processes subscribe request
func (s *WebSocketServer) handleSubscribe(client *WSClient, req *WSRequest) {
	// Return subscription info
	result := []interface{}{
		[][]string{
			{"mining.notify", "subscription"},
		},
		client.ID,
	}
	s.sendResult(client, req.ID, result)

	// Send difficulty
	s.sendNotify(client, "mining.set_difficulty", []interface{}{client.Difficulty})
}

// handleGetWork processes getwork request
func (s *WebSocketServer) handleGetWork(client *WSClient, req *WSRequest) {
	if !client.Authorized {
		s.sendError(client, req.ID, 24, "Unauthorized")
		return
	}

	job := s.getCurrentJob()
	if job == nil {
		s.sendError(client, req.ID, -1, "No work available")
		return
	}

	result := GetWorkResult{
		HeaderHash: job.HeaderHash,
		Target:     job.Target,
		Height:     job.Height,
		JobID:      job.ID,
		Difficulty: client.Difficulty,
	}

	s.sendResult(client, req.ID, result)
}

// handleSubmit processes share submission
func (s *WebSocketServer) handleSubmit(client *WSClient, req *WSRequest) {
	if !client.Authorized {
		s.sendError(client, req.ID, 24, "Unauthorized")
		return
	}

	if len(req.Params) < 2 {
		s.sendError(client, req.ID, -1, "Invalid params")
		return
	}

	// Parse params - can be [nonce, headerHash] or [jobId, nonce]
	var jobID, nonce string

	switch v := req.Params[0].(type) {
	case string:
		// Could be nonce or jobID
		if len(req.Params) >= 2 {
			if s, ok := req.Params[1].(string); ok && len(s) > 20 {
				// First param is nonce, second is headerHash
				nonce = v
				jobID = "" // Will use current job
			} else {
				// First param is jobID
				jobID = v
				nonce, _ = req.Params[1].(string)
			}
		}
	}

	job := s.getCurrentJob()
	if job == nil {
		s.sendError(client, req.ID, 21, "Job not found")
		return
	}

	if jobID == "" {
		jobID = job.ID
	}

	// Create share
	share := &Share{
		SessionID:  client.ID,
		Address:    client.Address,
		Worker:     client.Worker,
		JobID:      jobID,
		Nonce:      nonce,
		Difficulty: client.Difficulty,
		Height:     job.Height,
		Timestamp:  time.Now().Unix(),
	}

	// Callback
	if s.onShare != nil {
		s.onShare(share)
	}

	s.sendResult(client, req.ID, true)
}

// BroadcastJob sends a new job to all clients
func (s *WebSocketServer) BroadcastJob(job *Job) {
	s.currentJob.Store(job)

	s.clients.Range(func(key, value interface{}) bool {
		client := value.(*WSClient)
		if client.Authorized {
			s.sendJob(client, job)
		}
		return true
	})
}

// getCurrentJob returns the current job
func (s *WebSocketServer) getCurrentJob() *Job {
	if job := s.currentJob.Load(); job != nil {
		return job.(*Job)
	}
	return nil
}

// sendJob sends job notification
func (s *WebSocketServer) sendJob(client *WSClient, job *Job) {
	params := []interface{}{
		job.ID,
		job.HeaderHash,
		job.Target,
		job.Height,
		job.CleanJobs,
	}
	s.sendNotify(client, "mining.notify", params)
}

// sendResult sends a success response
func (s *WebSocketServer) sendResult(client *WSClient, id interface{}, result interface{}) {
	resp := WSResponse{
		ID:     id,
		Result: result,
	}
	s.send(client, resp)
}

// sendError sends an error response
func (s *WebSocketServer) sendError(client *WSClient, id interface{}, code int, message string) {
	resp := WSResponse{
		ID:    id,
		Error: []interface{}{code, message, nil},
	}
	s.send(client, resp)
}

// sendNotify sends a notification
func (s *WebSocketServer) sendNotify(client *WSClient, method string, params []interface{}) {
	notify := WSNotify{
		Method: method,
		Params: params,
	}
	s.send(client, notify)
}

// send writes a message to the client
func (s *WebSocketServer) send(client *WSClient, msg interface{}) {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()

	client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := client.Conn.WriteJSON(msg); err != nil {
		util.Debugf("WebSocket write error for client %d: %v", client.ID, err)
	}
}

// GetClientCount returns number of connected clients
func (s *WebSocketServer) GetClientCount() int {
	count := 0
	s.clients.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

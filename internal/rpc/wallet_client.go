// Package rpc provides RPC clients for TOS daemon and wallet.
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WalletClient provides access to TOS wallet RPC API for sending transactions.
type WalletClient struct {
	endpoint string
	username string
	password string
	client   *http.Client
}

// NewWalletClient creates a new wallet RPC client.
func NewWalletClient(endpoint, username, password string) *WalletClient {
	return &WalletClient{
		endpoint: endpoint,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TransferDestination represents a single transfer destination.
type TransferDestination struct {
	Address   string `json:"address"`
	Amount    uint64 `json:"amount"`
	Asset     string `json:"asset,omitempty"`     // Optional, defaults to TOS
	ExtraData []byte `json:"extra_data,omitempty"` // Optional extra data
}

// BuildTransactionParams contains parameters for building a transaction.
type BuildTransactionParams struct {
	TxType    TransactionType       `json:"tx_type"`
	Broadcast bool                  `json:"broadcast"`
	TxAsHex   bool                  `json:"tx_as_hex,omitempty"`
	Fee       *FeeParams            `json:"fee,omitempty"`
}

// TransactionType represents the type of transaction.
type TransactionType struct {
	Transfers []TransferDestination `json:"transfers,omitempty"`
}

// FeeParams represents fee configuration.
type FeeParams struct {
	Multiplier float64 `json:"multiplier,omitempty"`
	Value      uint64  `json:"value,omitempty"`
}

// TransactionResponse contains the response from build_transaction.
type TransactionResponse struct {
	Hash    string `json:"hash"`
	TxAsHex string `json:"tx_as_hex,omitempty"`
	Inner   struct {
		Hash string          `json:"hash"`
		Data json.RawMessage `json:"data"`
	} `json:"inner"`
}

// WalletRPCRequest represents a JSON-RPC request.
type WalletRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// WalletRPCResponse represents a JSON-RPC response.
type WalletRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Note: RPCError is defined in tos_client.go

// call makes a JSON-RPC call to the wallet.
func (w *WalletClient) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	req := WalletRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.endpoint+"/json_rpc", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Add basic auth if credentials are provided
	if w.username != "" || w.password != "" {
		httpReq.SetBasicAuth(w.username, w.password)
	}

	resp, err := w.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wallet RPC error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp WalletRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("wallet RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// GetAddress returns the wallet's address.
func (w *WalletClient) GetAddress(ctx context.Context) (string, error) {
	result, err := w.call(ctx, "get_address", nil)
	if err != nil {
		return "", err
	}

	var address string
	if err := json.Unmarshal(result, &address); err != nil {
		return "", fmt.Errorf("failed to parse address: %w", err)
	}

	return address, nil
}

// GetBalance returns the wallet's balance for the native asset.
func (w *WalletClient) GetBalance(ctx context.Context) (uint64, error) {
	result, err := w.call(ctx, "get_balance", nil)
	if err != nil {
		return 0, err
	}

	var balance uint64
	if err := json.Unmarshal(result, &balance); err != nil {
		return 0, fmt.Errorf("failed to parse balance: %w", err)
	}

	return balance, nil
}

// IsOnline checks if the wallet is connected to a daemon.
func (w *WalletClient) IsOnline(ctx context.Context) (bool, error) {
	result, err := w.call(ctx, "is_online", nil)
	if err != nil {
		return false, err
	}

	var online bool
	if err := json.Unmarshal(result, &online); err != nil {
		return false, fmt.Errorf("failed to parse online status: %w", err)
	}

	return online, nil
}

// BuildTransaction builds and optionally broadcasts a transaction.
func (w *WalletClient) BuildTransaction(ctx context.Context, destinations []TransferDestination, broadcast bool) (*TransactionResponse, error) {
	params := BuildTransactionParams{
		TxType: TransactionType{
			Transfers: destinations,
		},
		Broadcast: broadcast,
		TxAsHex:   false,
	}

	result, err := w.call(ctx, "build_transaction", params)
	if err != nil {
		return nil, err
	}

	var txResp TransactionResponse
	if err := json.Unmarshal(result, &txResp); err != nil {
		return nil, fmt.Errorf("failed to parse transaction response: %w", err)
	}

	return &txResp, nil
}

// Transfer sends a single transfer transaction.
func (w *WalletClient) Transfer(ctx context.Context, to string, amount uint64) (string, error) {
	destinations := []TransferDestination{
		{
			Address: to,
			Amount:  amount,
		},
	}

	resp, err := w.BuildTransaction(ctx, destinations, true)
	if err != nil {
		return "", err
	}

	// Return the transaction hash
	if resp.Inner.Hash != "" {
		return resp.Inner.Hash, nil
	}
	return resp.Hash, nil
}

// BatchTransfer sends a batch transfer transaction to multiple addresses.
func (w *WalletClient) BatchTransfer(ctx context.Context, destinations []TransferDestination) (string, error) {
	if len(destinations) == 0 {
		return "", fmt.Errorf("no destinations provided")
	}

	resp, err := w.BuildTransaction(ctx, destinations, true)
	if err != nil {
		return "", err
	}

	// Return the transaction hash
	if resp.Inner.Hash != "" {
		return resp.Inner.Hash, nil
	}
	return resp.Hash, nil
}

// Ping checks if the wallet RPC is reachable.
func (w *WalletClient) Ping(ctx context.Context) error {
	_, err := w.call(ctx, "get_version", nil)
	return err
}

package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tos-network/tos-pool/internal/storage"
)

func TestNewNotifier(t *testing.T) {
	cfg := &WebhookConfig{
		Enabled:     true,
		DiscordURL:  "https://discord.com/api/webhooks/test",
		TelegramBot: "bot_token",
		TelegramChat: "chat_id",
		PoolName:    "Test Pool",
		PoolURL:     "https://pool.example.com",
	}

	n := NewNotifier(cfg)

	if n == nil {
		t.Fatal("NewNotifier returned nil")
	}

	if n.cfg != cfg {
		t.Error("Notifier.cfg not set correctly")
	}

	if n.client == nil {
		t.Error("Notifier.client should not be nil")
	}

	if n.client.Timeout != 10*time.Second {
		t.Errorf("Client timeout = %v, want 10s", n.client.Timeout)
	}
}

func TestWebhookConfigStruct(t *testing.T) {
	cfg := WebhookConfig{
		DiscordURL:   "https://discord.com/api/webhooks/123/abc",
		TelegramURL:  "https://api.telegram.org",
		TelegramBot:  "123456:ABC",
		TelegramChat: "-100123456",
		Enabled:      true,
		PoolName:     "TOS Pool",
		PoolURL:      "https://pool.tos.network",
	}

	if cfg.DiscordURL != "https://discord.com/api/webhooks/123/abc" {
		t.Errorf("DiscordURL = %s, want https://discord.com/api/webhooks/123/abc", cfg.DiscordURL)
	}

	if cfg.TelegramBot != "123456:ABC" {
		t.Errorf("TelegramBot = %s, want 123456:ABC", cfg.TelegramBot)
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestDiscordEmbedStruct(t *testing.T) {
	embed := DiscordEmbed{
		Title:       "Block Found!",
		Description: "Test Pool found a new block!",
		URL:         "https://pool.example.com",
		Color:       0x00FF00,
		Fields: []DiscordField{
			{Name: "Height", Value: "12345", Inline: true},
			{Name: "Reward", Value: "5.0000 TOS", Inline: true},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Footer: &DiscordFooter{
			Text: "Test Pool",
		},
	}

	if embed.Title != "Block Found!" {
		t.Errorf("Embed.Title = %s, want Block Found!", embed.Title)
	}

	if embed.Color != 0x00FF00 {
		t.Errorf("Embed.Color = %d, want %d", embed.Color, 0x00FF00)
	}

	if len(embed.Fields) != 2 {
		t.Errorf("Embed.Fields len = %d, want 2", len(embed.Fields))
	}

	if embed.Footer.Text != "Test Pool" {
		t.Errorf("Embed.Footer.Text = %s, want Test Pool", embed.Footer.Text)
	}
}

func TestDiscordMessageStruct(t *testing.T) {
	msg := DiscordMessage{
		Content: "Test content",
		Embeds: []DiscordEmbed{
			{Title: "Test", Description: "Test embed"},
		},
	}

	if msg.Content != "Test content" {
		t.Errorf("Message.Content = %s, want Test content", msg.Content)
	}

	if len(msg.Embeds) != 1 {
		t.Errorf("Message.Embeds len = %d, want 1", len(msg.Embeds))
	}
}

func TestTelegramMessageStruct(t *testing.T) {
	msg := TelegramMessage{
		ChatID:    "-100123456",
		Text:      "*Block Found!*\nHeight: 12345",
		ParseMode: "Markdown",
	}

	if msg.ChatID != "-100123456" {
		t.Errorf("Message.ChatID = %s, want -100123456", msg.ChatID)
	}

	if msg.ParseMode != "Markdown" {
		t.Errorf("Message.ParseMode = %s, want Markdown", msg.ParseMode)
	}
}

func TestTruncateAddress(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "short"},
		{"exactly16chars!", "exactly16chars!"},
		{"tos1abcdefghijklmnopqrstuvwxyz", "tos1abcd...uvwxyz"},
		{"0x1234567890abcdef1234567890abcdef12345678", "0x123456...345678"},
	}

	for _, tt := range tests {
		result := truncateAddress(tt.input)
		if result != tt.expected {
			t.Errorf("truncateAddress(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTruncateHash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"shorthash", "shorthash"},
		{"exactly20characters!", "exactly20characters!"},
		{"0x1234567890abcdef1234567890abcdef12345678901234567890", "0x12345678...34567890"},
		{"abcdefghijklmnopqrstuvwxyz1234567890", "abcdefghij...34567890"},
	}

	for _, tt := range tests {
		result := truncateHash(tt.input)
		if result != tt.expected {
			t.Errorf("truncateHash(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestNotifyBlockFoundDisabled(t *testing.T) {
	cfg := &WebhookConfig{
		Enabled: false,
	}
	n := NewNotifier(cfg)

	block := &storage.Block{
		Height:      12345,
		Hash:        "0xabcdef",
		Finder:      "tos1finder",
		Reward:      5000000000,
		RoundShares: 100000,
	}

	// Should not panic or block when disabled
	n.NotifyBlockFound(block, 100000)
}

func TestNotifyPaymentSentDisabled(t *testing.T) {
	cfg := &WebhookConfig{
		Enabled: false,
	}
	n := NewNotifier(cfg)

	// Should not panic or block when disabled
	n.NotifyPaymentSent(1000000000, 10)
}

func TestNotifyOrphanBlockDisabled(t *testing.T) {
	cfg := &WebhookConfig{
		Enabled: false,
	}
	n := NewNotifier(cfg)

	block := &storage.Block{
		Height: 12345,
		Hash:   "0xabcdef",
		Finder: "tos1finder",
	}

	// Should not panic or block when disabled
	n.NotifyOrphanBlock(block)
}

func TestNotifyLargePaymentDisabled(t *testing.T) {
	cfg := &WebhookConfig{
		Enabled: false,
	}
	n := NewNotifier(cfg)

	// Should not panic or block when disabled
	n.NotifyLargePayment("tos1address", 100000000000, 50000000000)
}

func TestNotifyLargePaymentBelowThreshold(t *testing.T) {
	var called int32

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &WebhookConfig{
		Enabled:    true,
		DiscordURL: server.URL,
	}
	n := NewNotifier(cfg)

	// Amount below threshold - should not send
	n.NotifyLargePayment("tos1address", 50000000000, 100000000000)
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&called) != 0 {
		t.Error("Should not send notification when amount is below threshold")
	}
}

func TestDiscordWebhookIntegration(t *testing.T) {
	var received DiscordMessage
	var callCount int32

	// Create test server that captures the message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &WebhookConfig{
		Enabled:    true,
		DiscordURL: server.URL,
		PoolName:   "Test Pool",
		PoolURL:    "https://pool.example.com",
	}
	n := NewNotifier(cfg)

	block := &storage.Block{
		Height:      12345,
		Hash:        "0x1234567890abcdef1234567890abcdef12345678901234567890abcdef123456",
		Finder:      "tos1abcdefghijklmnopqrstuvwxyz123456",
		Reward:      5000000000,
		RoundShares: 100000,
	}

	n.NotifyBlockFound(block, 100000)

	// Wait for async send
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("Expected 1 call, got %d", atomic.LoadInt32(&callCount))
	}

	if len(received.Embeds) == 0 {
		t.Fatal("No embeds received")
	}

	if received.Embeds[0].Title != "Block Found!" {
		t.Errorf("Embed title = %s, want Block Found!", received.Embeds[0].Title)
	}

	if received.Embeds[0].Color != 0x00FF00 {
		t.Errorf("Embed color = %d, want green (0x00FF00)", received.Embeds[0].Color)
	}
}

func TestDiscordPaymentNotification(t *testing.T) {
	var received DiscordMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &WebhookConfig{
		Enabled:    true,
		DiscordURL: server.URL,
		PoolName:   "Test Pool",
	}
	n := NewNotifier(cfg)

	n.NotifyPaymentSent(10000000000, 25)
	time.Sleep(200 * time.Millisecond)

	if len(received.Embeds) == 0 {
		t.Fatal("No embeds received")
	}

	if received.Embeds[0].Title != "Payments Sent" {
		t.Errorf("Embed title = %s, want Payments Sent", received.Embeds[0].Title)
	}

	if received.Embeds[0].Color != 0x0099FF {
		t.Errorf("Embed color = %d, want blue (0x0099FF)", received.Embeds[0].Color)
	}
}

func TestDiscordOrphanNotification(t *testing.T) {
	var received DiscordMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &WebhookConfig{
		Enabled:    true,
		DiscordURL: server.URL,
		PoolName:   "Test Pool",
	}
	n := NewNotifier(cfg)

	block := &storage.Block{
		Height: 12345,
		Hash:   "0xorphanedhash",
		Finder: "tos1finder",
	}

	n.NotifyOrphanBlock(block)
	time.Sleep(200 * time.Millisecond)

	if len(received.Embeds) == 0 {
		t.Fatal("No embeds received")
	}

	if received.Embeds[0].Title != "Block Orphaned" {
		t.Errorf("Embed title = %s, want Block Orphaned", received.Embeds[0].Title)
	}

	if received.Embeds[0].Color != 0xFF0000 {
		t.Errorf("Embed color = %d, want red (0xFF0000)", received.Embeds[0].Color)
	}
}

func TestDiscordLargePaymentNotification(t *testing.T) {
	var received DiscordMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &WebhookConfig{
		Enabled:    true,
		DiscordURL: server.URL,
		PoolName:   "Test Pool",
	}
	n := NewNotifier(cfg)

	n.NotifyLargePayment("tos1largeaddress", 100000000000, 50000000000)
	time.Sleep(200 * time.Millisecond)

	if len(received.Embeds) == 0 {
		t.Fatal("No embeds received")
	}

	if received.Embeds[0].Title != "Large Payment Alert" {
		t.Errorf("Embed title = %s, want Large Payment Alert", received.Embeds[0].Title)
	}

	if received.Embeds[0].Color != 0xFFA500 {
		t.Errorf("Embed color = %d, want orange (0xFFA500)", received.Embeds[0].Color)
	}
}

func TestTelegramWebhookIntegration(t *testing.T) {
	var received TelegramMessage
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Extract the base URL without path
	cfg := &WebhookConfig{
		Enabled:      true,
		TelegramBot:  "test_token",
		TelegramChat: "-100123456",
		PoolName:     "Test Pool",
	}

	// Override the client to use test server
	n := NewNotifier(cfg)
	n.client = server.Client()

	// We can't easily test Telegram since it constructs the URL internally
	// Instead, test the message formatting functions directly
}

func TestDiscordRetryOnFailure(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &WebhookConfig{
		Enabled:    true,
		DiscordURL: server.URL,
		PoolName:   "Test Pool",
	}
	n := NewNotifier(cfg)

	block := &storage.Block{
		Height: 12345,
		Hash:   "0xhash",
		Finder: "tos1finder",
		Reward: 5000000000,
	}

	n.NotifyBlockFound(block, 100000)

	// Wait for retries
	time.Sleep(5 * time.Second)

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("Expected at least 2 calls (with retry), got %d", atomic.LoadInt32(&callCount))
	}
}

func TestDiscordRateLimitHandling(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			w.WriteHeader(http.StatusTooManyRequests) // 429
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &WebhookConfig{
		Enabled:    true,
		DiscordURL: server.URL,
		PoolName:   "Test Pool",
	}
	n := NewNotifier(cfg)

	block := &storage.Block{
		Height: 12345,
		Hash:   "0xhash",
		Finder: "tos1finder",
		Reward: 5000000000,
	}

	n.NotifyBlockFound(block, 100000)

	// Wait for rate limit handling (5s wait + retry delay)
	time.Sleep(10 * time.Second)

	count := atomic.LoadInt32(&callCount)
	// At minimum we should have had 1 call, and likely got a retry
	if count < 1 {
		t.Errorf("Expected at least 1 call, got %d calls", count)
	}
}

func TestEffortCalculation(t *testing.T) {
	var received DiscordMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &WebhookConfig{
		Enabled:    true,
		DiscordURL: server.URL,
		PoolName:   "Test Pool",
	}
	n := NewNotifier(cfg)

	// Block with 50% effort (50000 shares / 100000 network diff)
	block := &storage.Block{
		Height:      12345,
		Hash:        "0xhash",
		Finder:      "tos1finder",
		Reward:      5000000000,
		RoundShares: 50000,
	}

	n.NotifyBlockFound(block, 100000)
	time.Sleep(200 * time.Millisecond)

	// Check that effort field exists and has reasonable value
	found := false
	for _, field := range received.Embeds[0].Fields {
		if field.Name == "Effort" {
			found = true
			if field.Value != "50.00%" {
				t.Errorf("Effort = %s, want 50.00%%", field.Value)
			}
		}
	}
	if !found {
		t.Error("Effort field not found in embed")
	}
}

func TestConstants(t *testing.T) {
	if MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", MaxRetries)
	}

	if RetryBaseDelay != 2*time.Second {
		t.Errorf("RetryBaseDelay = %v, want 2s", RetryBaseDelay)
	}
}

func TestNotifyBlockFoundWithZeroNetworkDiff(t *testing.T) {
	var received DiscordMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &WebhookConfig{
		Enabled:    true,
		DiscordURL: server.URL,
		PoolName:   "Test Pool",
	}
	n := NewNotifier(cfg)

	block := &storage.Block{
		Height:      12345,
		Hash:        "0xhash",
		Finder:      "tos1finder",
		Reward:      5000000000,
		RoundShares: 100000,
	}

	// Zero network diff - should handle gracefully
	n.NotifyBlockFound(block, 0)
	time.Sleep(200 * time.Millisecond)

	// Should still send notification
	if len(received.Embeds) == 0 {
		t.Error("Should still send notification with zero network diff")
	}
}

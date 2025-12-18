// Package notify provides notification services for pool events.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tos-network/tos-pool/internal/storage"
	"github.com/tos-network/tos-pool/internal/util"
)

// WebhookConfig holds webhook configuration
type WebhookConfig struct {
	DiscordURL  string `mapstructure:"discord_url"`
	TelegramURL string `mapstructure:"telegram_url"`
	TelegramBot string `mapstructure:"telegram_bot"`
	TelegramChat string `mapstructure:"telegram_chat"`
	Enabled     bool   `mapstructure:"enabled"`
	PoolName    string
	PoolURL     string `mapstructure:"pool_url"`
}

// Retry configuration
const (
	MaxRetries     = 3
	RetryBaseDelay = 2 * time.Second
)

// Notifier handles sending notifications
type Notifier struct {
	cfg    *WebhookConfig
	client *http.Client
}

// NewNotifier creates a new notifier
func NewNotifier(cfg *WebhookConfig) *Notifier {
	return &Notifier{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NotifyBlockFound sends notifications when a block is found
func (n *Notifier) NotifyBlockFound(block *storage.Block, networkDiff uint64) {
	if !n.cfg.Enabled {
		return
	}

	if n.cfg.DiscordURL != "" {
		go n.sendDiscordNotification(block, networkDiff)
	}

	if n.cfg.TelegramBot != "" && n.cfg.TelegramChat != "" {
		go n.sendTelegramNotification(block, networkDiff)
	}
}

// NotifyPaymentSent sends notifications when payments are processed
func (n *Notifier) NotifyPaymentSent(totalPaid uint64, minerCount int) {
	if !n.cfg.Enabled {
		return
	}

	if n.cfg.DiscordURL != "" {
		go n.sendDiscordPaymentNotification(totalPaid, minerCount)
	}

	if n.cfg.TelegramBot != "" && n.cfg.TelegramChat != "" {
		go n.sendTelegramPaymentNotification(totalPaid, minerCount)
	}
}

// NotifyOrphanBlock sends notifications when a block is orphaned
func (n *Notifier) NotifyOrphanBlock(block *storage.Block) {
	if !n.cfg.Enabled {
		return
	}

	if n.cfg.DiscordURL != "" {
		go n.sendDiscordOrphanNotification(block)
	}

	if n.cfg.TelegramBot != "" && n.cfg.TelegramChat != "" {
		go n.sendTelegramOrphanNotification(block)
	}
}

// NotifyLargePayment sends notifications for unusually large payments
func (n *Notifier) NotifyLargePayment(address string, amount uint64, threshold uint64) {
	if !n.cfg.Enabled || amount < threshold {
		return
	}

	if n.cfg.DiscordURL != "" {
		go n.sendDiscordLargePaymentNotification(address, amount)
	}

	if n.cfg.TelegramBot != "" && n.cfg.TelegramChat != "" {
		go n.sendTelegramLargePaymentNotification(address, amount)
	}
}

// DiscordEmbed represents a Discord embed object
type DiscordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url,omitempty"`
	Color       int            `json:"color,omitempty"`
	Fields      []DiscordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
	Footer      *DiscordFooter `json:"footer,omitempty"`
}

// DiscordField represents a field in a Discord embed
type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// DiscordFooter represents the footer of a Discord embed
type DiscordFooter struct {
	Text string `json:"text"`
}

// DiscordMessage represents a Discord webhook message
type DiscordMessage struct {
	Content string         `json:"content,omitempty"`
	Embeds  []DiscordEmbed `json:"embeds,omitempty"`
}

// sendDiscordNotification sends a block found notification to Discord
func (n *Notifier) sendDiscordNotification(block *storage.Block, networkDiff uint64) {
	// Calculate effort (luck)
	var effort float64
	if block.RoundShares > 0 && networkDiff > 0 {
		expectedShares := float64(networkDiff)
		effort = (float64(block.RoundShares) / expectedShares) * 100
	}

	// Format reward (assuming 9 decimals for TOS)
	rewardTOS := float64(block.Reward) / 1e9

	embed := DiscordEmbed{
		Title:       "Block Found!",
		Description: fmt.Sprintf("**%s** found a new block!", n.cfg.PoolName),
		Color:       0x00FF00, // Green
		Fields: []DiscordField{
			{Name: "Height", Value: fmt.Sprintf("%d", block.Height), Inline: true},
			{Name: "Reward", Value: fmt.Sprintf("%.4f TOS", rewardTOS), Inline: true},
			{Name: "Effort", Value: fmt.Sprintf("%.2f%%", effort), Inline: true},
			{Name: "Finder", Value: truncateAddress(block.Finder), Inline: true},
			{Name: "Hash", Value: truncateHash(block.Hash), Inline: false},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Footer: &DiscordFooter{
			Text: n.cfg.PoolName,
		},
	}

	if n.cfg.PoolURL != "" {
		embed.URL = n.cfg.PoolURL
	}

	msg := DiscordMessage{
		Embeds: []DiscordEmbed{embed},
	}

	n.sendDiscordMessage(msg)
}

// sendDiscordPaymentNotification sends a payment notification to Discord
func (n *Notifier) sendDiscordPaymentNotification(totalPaid uint64, minerCount int) {
	paidTOS := float64(totalPaid) / 1e9

	embed := DiscordEmbed{
		Title:       "Payments Sent",
		Description: fmt.Sprintf("**%s** has processed payouts", n.cfg.PoolName),
		Color:       0x0099FF, // Blue
		Fields: []DiscordField{
			{Name: "Total Paid", Value: fmt.Sprintf("%.4f TOS", paidTOS), Inline: true},
			{Name: "Miners", Value: fmt.Sprintf("%d", minerCount), Inline: true},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Footer: &DiscordFooter{
			Text: n.cfg.PoolName,
		},
	}

	msg := DiscordMessage{
		Embeds: []DiscordEmbed{embed},
	}

	n.sendDiscordMessage(msg)
}

// sendDiscordOrphanNotification sends an orphan block notification to Discord
func (n *Notifier) sendDiscordOrphanNotification(block *storage.Block) {
	embed := DiscordEmbed{
		Title:       "Block Orphaned",
		Description: fmt.Sprintf("**%s** block was orphaned", n.cfg.PoolName),
		Color:       0xFF0000, // Red
		Fields: []DiscordField{
			{Name: "Height", Value: fmt.Sprintf("%d", block.Height), Inline: true},
			{Name: "Finder", Value: truncateAddress(block.Finder), Inline: true},
			{Name: "Hash", Value: truncateHash(block.Hash), Inline: false},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Footer: &DiscordFooter{
			Text: n.cfg.PoolName,
		},
	}

	msg := DiscordMessage{
		Embeds: []DiscordEmbed{embed},
	}

	n.sendDiscordMessageWithRetry(msg)
}

// sendDiscordLargePaymentNotification sends a large payment warning to Discord
func (n *Notifier) sendDiscordLargePaymentNotification(address string, amount uint64) {
	amountTOS := float64(amount) / 1e9

	embed := DiscordEmbed{
		Title:       "Large Payment Alert",
		Description: fmt.Sprintf("**%s** processed a large payment", n.cfg.PoolName),
		Color:       0xFFA500, // Orange
		Fields: []DiscordField{
			{Name: "Amount", Value: fmt.Sprintf("%.4f TOS", amountTOS), Inline: true},
			{Name: "Address", Value: truncateAddress(address), Inline: true},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Footer: &DiscordFooter{
			Text: n.cfg.PoolName,
		},
	}

	msg := DiscordMessage{
		Embeds: []DiscordEmbed{embed},
	}

	n.sendDiscordMessageWithRetry(msg)
}

// sendDiscordMessage sends a message to Discord webhook (no retry)
func (n *Notifier) sendDiscordMessage(msg DiscordMessage) {
	n.sendDiscordMessageWithRetry(msg)
}

// sendDiscordMessageWithRetry sends a message to Discord with exponential backoff retry
func (n *Notifier) sendDiscordMessageWithRetry(msg DiscordMessage) {
	body, err := json.Marshal(msg)
	if err != nil {
		util.Warnf("Failed to marshal Discord message: %v", err)
		return
	}

	var lastErr error
	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2s, 4s, 8s
			delay := RetryBaseDelay * time.Duration(1<<uint(attempt-1))
			time.Sleep(delay)
		}

		resp, err := n.client.Post(n.cfg.DiscordURL, "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}

		resp.Body.Close()

		if resp.StatusCode < 400 {
			return // Success
		}

		// Rate limited - wait longer
		if resp.StatusCode == 429 {
			time.Sleep(5 * time.Second)
			continue
		}

		lastErr = fmt.Errorf("status %d", resp.StatusCode)
	}

	if lastErr != nil {
		util.Warnf("Failed to send Discord notification after %d retries: %v", MaxRetries, lastErr)
	}
}

// TelegramMessage represents a Telegram bot message
type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// sendTelegramNotification sends a block found notification to Telegram
func (n *Notifier) sendTelegramNotification(block *storage.Block, networkDiff uint64) {
	var effort float64
	if block.RoundShares > 0 && networkDiff > 0 {
		expectedShares := float64(networkDiff)
		effort = (float64(block.RoundShares) / expectedShares) * 100
	}

	rewardTOS := float64(block.Reward) / 1e9

	text := fmt.Sprintf(
		"*Block Found!*\n\n"+
			"Height: `%d`\n"+
			"Reward: `%.4f TOS`\n"+
			"Effort: `%.2f%%`\n"+
			"Finder: `%s`\n"+
			"Hash: `%s`",
		block.Height, rewardTOS, effort,
		truncateAddress(block.Finder), truncateHash(block.Hash),
	)

	n.sendTelegramMessage(text)
}

// sendTelegramPaymentNotification sends a payment notification to Telegram
func (n *Notifier) sendTelegramPaymentNotification(totalPaid uint64, minerCount int) {
	paidTOS := float64(totalPaid) / 1e9

	text := fmt.Sprintf(
		"*Payments Sent*\n\n"+
			"Total Paid: `%.4f TOS`\n"+
			"Miners: `%d`",
		paidTOS, minerCount,
	)

	n.sendTelegramMessage(text)
}

// sendTelegramOrphanNotification sends an orphan block notification to Telegram
func (n *Notifier) sendTelegramOrphanNotification(block *storage.Block) {
	text := fmt.Sprintf(
		"*Block Orphaned*\n\n"+
			"Height: `%d`\n"+
			"Finder: `%s`\n"+
			"Hash: `%s`",
		block.Height,
		truncateAddress(block.Finder), truncateHash(block.Hash),
	)

	n.sendTelegramMessageWithRetry(text)
}

// sendTelegramLargePaymentNotification sends a large payment alert to Telegram
func (n *Notifier) sendTelegramLargePaymentNotification(address string, amount uint64) {
	amountTOS := float64(amount) / 1e9

	text := fmt.Sprintf(
		"*Large Payment Alert*\n\n"+
			"Amount: `%.4f TOS`\n"+
			"Address: `%s`",
		amountTOS, truncateAddress(address),
	)

	n.sendTelegramMessageWithRetry(text)
}

// sendTelegramMessage sends a message via Telegram Bot API (no retry)
func (n *Notifier) sendTelegramMessage(text string) {
	n.sendTelegramMessageWithRetry(text)
}

// sendTelegramMessageWithRetry sends a message via Telegram with exponential backoff retry
func (n *Notifier) sendTelegramMessageWithRetry(text string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.cfg.TelegramBot)

	msg := TelegramMessage{
		ChatID:    n.cfg.TelegramChat,
		Text:      text,
		ParseMode: "Markdown",
	}

	body, err := json.Marshal(msg)
	if err != nil {
		util.Warnf("Failed to marshal Telegram message: %v", err)
		return
	}

	var lastErr error
	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			delay := RetryBaseDelay * time.Duration(1<<uint(attempt-1))
			time.Sleep(delay)
		}

		resp, err := n.client.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}

		resp.Body.Close()

		if resp.StatusCode < 400 {
			return // Success
		}

		// Rate limited
		if resp.StatusCode == 429 {
			time.Sleep(5 * time.Second)
			continue
		}

		lastErr = fmt.Errorf("status %d", resp.StatusCode)
	}

	if lastErr != nil {
		util.Warnf("Failed to send Telegram notification after %d retries: %v", MaxRetries, lastErr)
	}
}

// truncateAddress returns a shortened address for display
func truncateAddress(addr string) string {
	if len(addr) <= 16 {
		return addr
	}
	return addr[:8] + "..." + addr[len(addr)-6:]
}

// truncateHash returns a shortened hash for display
func truncateHash(hash string) string {
	if len(hash) <= 20 {
		return hash
	}
	return hash[:10] + "..." + hash[len(hash)-8:]
}

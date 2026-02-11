package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type BotClient struct {
	token   string
	baseURL string
	bot     *bot.Bot
}

func NewBotClient(token, baseURL string) (*BotClient, error) {
	client := &BotClient{
		token:   strings.TrimSpace(token),
		baseURL: strings.TrimRight(baseURL, "/"),
	}

	if client.token == "" {
		return client, nil
	}

	b, err := bot.New(client.token)
	if err != nil {
		return nil, fmt.Errorf("telegram bot init failed: %w", err)
	}
	client.bot = b
	if err := client.ensureWebhook(); err != nil {
		log.Printf("telegram webhook setup failed: %v", err)
	}

	startRegex := regexp.MustCompile(`^/start(?:\s+(.+))?$`)
	inviteRegex := regexp.MustCompile(`^/invite\s+([A-Za-z0-9_-]+)$`)
	simRegex := regexp.MustCompile(`^/sim(?:ulator)?\s+([A-Za-z0-9_-]+)$`)
	client.bot.RegisterHandlerRegexp(bot.HandlerTypeMessageText, startRegex, client.handleStart)
	client.bot.RegisterHandlerRegexp(bot.HandlerTypeMessageText, inviteRegex, client.handleInviteCmd)
	client.bot.RegisterHandlerRegexp(bot.HandlerTypeMessageText, simRegex, client.handleSimulatorCmd)
	client.bot.RegisterHandler(bot.HandlerTypeMessageText, "/health", bot.MatchTypeExact, client.handleHealth)
	client.bot.RegisterHandler(bot.HandlerTypeMessageText, "/debug", bot.MatchTypeExact, client.handleDebug)
	client.bot.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, client.handleHelp)
	client.bot.RegisterHandler(bot.HandlerTypeMessageText, "/myid", bot.MatchTypeExact, client.handleMyID)

	log.Println("Telegram bot initialized with go-telegram/bot")
	return client, nil
}

func (b *BotClient) ensureWebhook() error {
	if b.token == "" || b.baseURL == "" {
		return nil
	}
	base := strings.TrimRight(strings.TrimSpace(b.baseURL), "/")
	if base == "" || strings.Contains(base, "localhost") || strings.HasPrefix(base, "http://") {
		return fmt.Errorf("BASE_URL must be public https URL for telegram webhook, got %q", b.baseURL)
	}
	webhookURL := base + "/api/telegram/webhook"

	form := url.Values{}
	form.Set("url", webhookURL)
	setResp, err := http.PostForm(fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", b.token), form)
	if err != nil {
		return fmt.Errorf("setWebhook request failed: %w", err)
	}
	defer setResp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(setResp.Body, 8*1024))
	if setResp.StatusCode < 200 || setResp.StatusCode >= 300 {
		return fmt.Errorf("setWebhook http=%d body=%s", setResp.StatusCode, string(body))
	}

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(body, &result)
	if !result.OK {
		return fmt.Errorf("setWebhook failed: %s", result.Description)
	}

	log.Printf("telegram webhook set to %s", webhookURL)
	return nil
}

func (b *BotClient) SendMessage(ctx context.Context, chatID, text string) error {
	if b.bot == nil || strings.TrimSpace(chatID) == "" || strings.TrimSpace(text) == "" {
		return nil
	}

	id, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat id: %w", err)
	}

	sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = b.bot.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID: id,
		Text:   text,
	})
	return err
}

func (b *BotClient) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if b.bot == nil {
		log.Println("telegram webhook received but bot is not initialized")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	update := models.Update{}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		log.Printf("telegram webhook decode failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if update.Message != nil {
		log.Printf("telegram update message received: chat_id=%d text=%q", update.Message.Chat.ID, update.Message.Text)
	}

	processCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	b.bot.ProcessUpdate(processCtx, &update)
	w.WriteHeader(http.StatusNoContent)
}

func (b *BotClient) handleStart(ctx context.Context, tg *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}

	text := strings.TrimSpace(update.Message.Text)
	parts := strings.SplitN(text, " ", 2)
	payload := ""
	if len(parts) == 2 {
		payload = strings.TrimSpace(parts[1])
	}

	chatID := update.Message.Chat.ID
	if strings.HasPrefix(strings.ToLower(payload), "invite_") {
		code := strings.TrimPrefix(payload, "invite_")
		b.sendInvitePack(tg, chatID, code)
		return
	}

	sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := tg.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID: chatID,
		Text: "CS2 LiveDrop bot is online.\n\n" +
			"Commands:\n" +
			"/myid - get this chat id\n" +
			"/invite CODE - get invite + Steam links\n" +
			"/sim CODE - open simulator invite link\n" +
			"/health - connectivity check\n" +
			"/debug - bot diagnostics",
	})
	if err != nil {
		log.Printf("telegram send start message failed: %v", err)
	}
}

func (b *BotClient) handleInviteCmd(ctx context.Context, tg *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	parts := strings.Fields(strings.TrimSpace(update.Message.Text))
	if len(parts) < 2 {
		return
	}
	b.sendInvitePack(tg, update.Message.Chat.ID, parts[1])
}

func (b *BotClient) handleSimulatorCmd(ctx context.Context, tg *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	parts := strings.Fields(strings.TrimSpace(update.Message.Text))
	if len(parts) < 2 {
		return
	}
	code := strings.TrimSpace(parts[1])
	simulatorURL := fmt.Sprintf("%s/simulator.html?invite=%s", b.baseURL, code)
	sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := tg.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Open simulator: " + simulatorURL,
	})
	if err != nil {
		log.Printf("telegram send /sim failed: %v", err)
	}
}

func (b *BotClient) handleHealth(ctx context.Context, tg *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := tg.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "ok",
	})
	if err != nil {
		log.Printf("telegram send /health failed: %v", err)
	}
}

func (b *BotClient) handleDebug(ctx context.Context, tg *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}

	tokenConfigured := b.token != ""
	sanitizedBaseURL := b.baseURL
	if sanitizedBaseURL == "" {
		sanitizedBaseURL = "(empty)"
	}
	debugText := fmt.Sprintf(
		"bot_debug\nlibrary=github.com/go-telegram/bot\nbase_url=%s\ntoken_configured=%t\ntime_utc=%s",
		sanitizedBaseURL,
		tokenConfigured,
		time.Now().UTC().Format(time.RFC3339),
	)
	sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := tg.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   debugText,
	})
	if err != nil {
		log.Printf("telegram send /debug failed: %v", err)
	}
}

func (b *BotClient) handleHelp(ctx context.Context, tg *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := tg.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text: "Commands:\n" +
			"/myid\n" +
			"/invite CODE\n" +
			"/sim CODE\n" +
			"/start invite_CODE\n" +
			"/health\n" +
			"/debug",
	})
	if err != nil {
		log.Printf("telegram send /help failed: %v", err)
	}
}

func (b *BotClient) handleMyID(ctx context.Context, tg *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := tg.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("chat_id=%d", update.Message.Chat.ID),
	})
	if err != nil {
		log.Printf("telegram send /myid failed: %v", err)
	}
}

func (b *BotClient) sendInvitePack(tg *bot.Bot, chatID int64, code string) {
	inviteURL := fmt.Sprintf("%s/invite/%s", b.baseURL, code)
	steamURL := fmt.Sprintf("%s/api/auth/steam/login?invite=%s", b.baseURL, code)
	simulatorURL := fmt.Sprintf("%s/simulator.html?invite=%s", b.baseURL, code)
	sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := tg.SendMessage(sendCtx, &bot.SendMessageParams{
		ChatID: chatID,
		Text: "Invite code: " + code + "\n" +
			"Invite: " + inviteURL + "\n" +
			"Steam login: " + steamURL + "\n" +
			"Simulator: " + simulatorURL,
	})
	if err != nil {
		log.Printf("telegram send invite pack failed: %v", err)
	}
}

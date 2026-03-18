package bot

import (
	"log"

	"expense-bot/internal/config"
	"expense-bot/internal/sheets"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot is the top-level application struct.
type Bot struct {
	api     *tgbotapi.BotAPI
	handler *Handler
}

// New creates and wires up the bot with all its dependencies.
func New(cfg *config.Config, sheetsClient *sheets.Client) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}
	log.Printf("Authorized as @%s", api.Self.UserName)

	h := newHandler(api, sheetsClient, cfg.Salary, cfg.SpreadsheetID, cfg.AllowedUserIDs)
	return &Bot{api: api, handler: h}, nil
}

// Run starts the polling loop and blocks until it stops.
func (b *Bot) Run() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	log.Println("🤖 Bot is running. Press Ctrl+C to stop.")
	for update := range updates {
		if update.Message != nil {
			go b.handler.HandleMessage(update.Message)
		}
		if update.CallbackQuery != nil {
			go b.handler.HandleCallback(update.CallbackQuery)
		}
	}
}

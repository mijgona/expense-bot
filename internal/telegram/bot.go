package telegram

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"expense-bot/internal/config"
	"expense-bot/internal/store"
)

// Bot is the minimal chat bot: /start registers the user and offers the Mini App;
// everything else points to the app (contracts/bot.md).
type Bot struct {
	c     *Client
	store store.Store
	cfg   *config.Config
}

func NewBot(c *Client, st store.Store, cfg *config.Config) *Bot {
	return &Bot{c: c, store: st, cfg: cfg}
}

// Start verifies the token, sets the menu button and long-polls until ctx is done.
func (b *Bot) Start(ctx context.Context) {
	name, err := b.c.GetMe(ctx)
	if err != nil {
		log.Fatalf("telegram: getMe: %v", err)
	}
	log.Printf("Authorized as @%s", name)

	if b.cfg.WebAppURL != "" {
		if err := b.c.SetChatMenuButton(ctx, "Открыть", b.cfg.WebAppURL); err != nil {
			log.Printf("webapp: set menu button: %v", err)
		} else {
			log.Printf("webapp: menu button set to %s", b.cfg.WebAppURL)
		}
	} else {
		log.Println("webapp: WEBAPP_URL not set, menu button skipped")
	}

	log.Println("Bot is running.")
	var offset int64
	for ctx.Err() == nil {
		ups, err := b.c.GetUpdates(ctx, offset, 30)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("telegram: getUpdates: %v", err) // includes 409 during zero-downtime deploys
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range ups {
			offset = u.UpdateID + 1
			if u.Message != nil {
				b.handle(ctx, u.Message)
			}
		}
	}
}

func (b *Bot) handle(ctx context.Context, m *Message) {
	if m.Chat.Type != "private" || m.From == nil {
		return
	}
	chatID := m.Chat.ID
	if len(b.cfg.AllowedUserIDs) > 0 && !slices.Contains(b.cfg.AllowedUserIDs, m.From.ID) {
		b.send(ctx, chatID, "⛔ Доступ запрещён.", false)
		return
	}

	if strings.HasPrefix(m.Text, "/start") {
		from := *m.From
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if _, _, err := b.store.EnsureUser(ctx, from.ID, from.FirstName, from.Username); err != nil {
				log.Printf("telegram: EnsureUser %d: %v", from.ID, err)
			}
		}()
		b.send(ctx, chatID, fmt.Sprintf(
			"👋 Привет, *%s*!\n\nВсе расходы, отчёты, накопления и цели теперь в приложении 👇",
			escapeMarkdown(m.From.FirstName)), true)
		return
	}
	b.send(ctx, chatID, "Бот теперь работает через приложение 👇", true)
}

func (b *Bot) send(ctx context.Context, chatID int64, text string, withButton bool) {
	url := ""
	if withButton {
		if b.cfg.WebAppURL == "" {
			text += "\n\nПриложение ещё не настроено."
		} else {
			url = b.cfg.WebAppURL
		}
	}
	if err := b.c.SendMessage(ctx, chatID, text, url); err != nil {
		log.Printf("telegram: send to %d: %v", chatID, err)
	}
}

// escapeMarkdown escapes legacy-Markdown control characters in user-provided text.
func escapeMarkdown(s string) string {
	return strings.NewReplacer("_", "\\_", "*", "\\*", "`", "\\`", "[", "\\[").Replace(s)
}

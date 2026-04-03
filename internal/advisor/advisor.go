package advisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"expense-bot/internal/category"
	"expense-bot/internal/sheets"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	geminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-pro-002:generateContent"
	interval     = 72 * time.Hour
)

// Advisor runs a periodic financial analysis for all users and sends results to Telegram.
type Advisor struct {
	api    *tgbotapi.BotAPI
	sheets *sheets.Client
	apiKey string
	salary int
}

// New creates an Advisor. apiKey is the Anthropic API key.
func New(api *tgbotapi.BotAPI, sheetsClient *sheets.Client, apiKey string, salary int) *Advisor {
	return &Advisor{
		api:    api,
		sheets: sheetsClient,
		apiKey: apiKey,
		salary: salary,
	}
}

// Start runs the advisory loop in the background every 3 days.
// Call as: go advisor.Start()
func (a *Advisor) Start() {
	if a.apiKey == "" {
		log.Println("advisor: GEMINI_API_KEY not set, skipping")
		return
	}
	log.Printf("advisor: started, interval=%v", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		a.runAll()
	}
}

func (a *Advisor) runAll() {
	users, err := a.sheets.GetUsers()
	if err != nil {
		log.Printf("advisor: get users: %v", err)
		return
	}
	for _, uid := range users {
		if err := a.runForUser(uid); err != nil {
			log.Printf("advisor: user %d: %v", uid, err)
		}
	}
}

// RunForUser triggers an immediate analysis for a single user.
func (a *Advisor) RunForUser(userID int64) error {
	if a.apiKey == "" {
		return fmt.Errorf("GEMINI_API_KEY not set")
	}
	return a.runForUser(userID)
}

func (a *Advisor) runForUser(userID int64) error {
	monthKey := time.Now().Format("2006-01")

	stats, err := a.sheets.GetMonthStats(userID, monthKey)
	if err != nil {
		return fmt.Errorf("month stats: %w", err)
	}
	savings, err := a.sheets.GetSavingsBalance(userID)
	if err != nil {
		return fmt.Errorf("savings balance: %w", err)
	}
	goals, err := a.sheets.GetGoals(userID)
	if err != nil {
		return fmt.Errorf("goals: %w", err)
	}

	prompt := buildPrompt(monthKey, a.salary, stats, savings, goals)

	advice, err := a.callGemini(prompt)
	if err != nil {
		return fmt.Errorf("claude: %w", err)
	}

	msg := tgbotapi.NewMessage(userID, "📊 *Финансовый отчёт за 3 дня*\n\n"+advice)
	msg.ParseMode = "Markdown"
	_, err = a.api.Send(msg)
	return err
}

func buildPrompt(monthKey string, salary int, stats *sheets.MonthStats, savings float64, goals []sheets.Goal) string {
	var sb strings.Builder

	sb.WriteString("Ты финансовый советник. Проанализируй финансовое положение и дай конкретные советы на русском языке.\n\n")
	fmt.Fprintf(&sb, "ПЕРИОД: %s | ЗАРПЛАТА: %d с.\n\n", monthKey, salary)

	sb.WriteString("РАСХОДЫ ПО КАТЕГОРИЯМ:\n")
	for _, cat := range category.All() {
		spent := stats.Totals[cat.Name]
		if spent == 0 && cat.Limit == 0 {
			continue
		}
		pct := 0.0
		if cat.Limit > 0 {
			pct = spent / float64(cat.Limit) * 100
		}
		fmt.Fprintf(&sb, "- %s: %.0f с. / %d с. лимит (%.0f%%)\n", cat.Label, spent, cat.Limit, pct)
	}
	fmt.Fprintf(&sb, "\nИТОГО РАСХОДЫ: %.0f с. | ДОХОДЫ: %.0f с.\n", stats.TotalExpense, stats.TotalIncome)
	fmt.Fprintf(&sb, "БАЛАНС НАКОПЛЕНИЙ (всё время): %.0f с.\n\n", savings)

	if len(goals) > 0 {
		sb.WriteString("КВАРТАЛЬНЫЕ ЦЕЛИ:\n")
		for _, g := range goals {
			status := g.Status
			if status == "" {
				status = "Активна"
			}
			fmt.Fprintf(&sb, "- %s: цель %.0f с., квартал %s, статус: %s\n",
				g.Name, g.TargetAmount, g.Quarter, status)
			if g.Description != "" {
				fmt.Fprintf(&sb, "  %s\n", g.Description)
			}
		}
		fmt.Fprintf(&sb, "Текущий баланс накоплений (общий): %.0f с.\n", savings)
	} else {
		sb.WriteString("ЦЕЛИ: не заданы.\n")
	}

	sb.WriteString(`
КОНТЕКСТ: инфляция в Таджикистане ~7-9% годовых. Учитывай реальную доходность.

Дай анализ (до 400 слов):
1. Прогресс по целям — реалистичен ли дедлайн при текущем темпе?
2. Какие статьи расходов превышены и что сократить?
3. Конкретные советы по реинвестированию с учётом инфляции (депозиты в Таджикистане, активы).
4. Итог: достигнет ли пользователь целей? Что изменить?

Пиши конкретно, с цифрами, без воды.`)

	return sb.String()
}

// ── Gemini API ────────────────────────────────────────────────────────────────

type geminiRequest struct {
	Contents         []geminiContent `json:"contents"`
	GenerationConfig geminiGenConfig `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (a *Advisor) callGemini(prompt string) (string, error) {
	body, err := json.Marshal(geminiRequest{
		Contents:         []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
		GenerationConfig: geminiGenConfig{MaxOutputTokens: 1024},
	})
	if err != nil {
		return "", err
	}

	url := geminiAPIURL + "?key=" + a.apiKey
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var gr geminiResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if gr.Error != nil {
			msg += ": " + gr.Error.Message
		}
		return "", fmt.Errorf("%s", msg)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return gr.Candidates[0].Content.Parts[0].Text, nil
}

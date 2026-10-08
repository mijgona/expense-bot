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
	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

const (
	geminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"
	interval     = 72 * time.Hour
)

// Notifier delivers a generated report (Markdown file) to the user, e.g. as a Telegram document.
type Notifier func(ctx context.Context, userID int64, filename string, md []byte) error

// Advisor produces Gemini-based financial reports: on demand (API) and every 72h (chat).
type Advisor struct {
	store  store.Store
	apiKey string
	salary int
	notify Notifier
}

// New creates an Advisor. apiKey is the Gemini API key; notify may be nil (no periodic delivery).
func New(st store.Store, apiKey string, salary int, notify Notifier) *Advisor {
	return &Advisor{store: st, apiKey: apiKey, salary: salary, notify: notify}
}

// Enabled reports whether GEMINI_API_KEY is configured.
func (a *Advisor) Enabled() bool { return a.apiKey != "" }

// Start runs the periodic loop until ctx is done. Call as: go adv.Start(ctx)
func (a *Advisor) Start(ctx context.Context) {
	if !a.Enabled() {
		log.Println("advisor: GEMINI_API_KEY not set, skipping")
		return
	}
	log.Printf("advisor: started, interval=%v", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runAll(ctx)
		}
	}
}

func (a *Advisor) runAll(ctx context.Context) {
	if a.notify == nil {
		return
	}
	users, err := a.store.ListUserIDs(ctx)
	if err != nil {
		log.Printf("advisor: list users: %v", err)
		return
	}
	for _, uid := range users {
		md, err := a.Generate(ctx, uid)
		if err != nil {
			log.Printf("advisor: user %d: %v", uid, err)
			continue
		}
		name := "report_" + ledger.Now().Format("2006-01-02") + ".md"
		if err := a.notify(ctx, uid, name, []byte(md)); err != nil {
			log.Printf("advisor: deliver to %d: %v", uid, err)
		}
	}
}

// Generate builds the report for one user and returns it as Markdown.
func (a *Advisor) Generate(ctx context.Context, userID int64) (string, error) {
	if !a.Enabled() {
		return "", fmt.Errorf("GEMINI_API_KEY not set")
	}
	monthKey := ledger.MonthKey(ledger.Now())
	month, err := a.store.GetMonth(ctx, userID, monthKey)
	if err != nil {
		return "", fmt.Errorf("month: %w", err)
	}
	u, err := a.store.GetUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("user: %w", err)
	}
	goals, err := a.store.ListGoals(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("goals: %w", err)
	}
	advice, err := a.callGemini(ctx, buildPrompt(monthKey, a.salary, month, u.SavingsBalance, goals))
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}
	return advice, nil
}

// somoni converts diram to somoni for prompt text.
func somoni(d int64) float64 { return float64(d) / ledger.PerSomoni }

func buildPrompt(monthKey string, salary int, m ledger.Month, savings int64, goals []store.Goal) string {
	var sb strings.Builder

	sb.WriteString("Ты финансовый советник. Проанализируй финансовое положение и дай конкретные советы на русском языке.\n\n")
	fmt.Fprintf(&sb, "ПЕРИОД: %s | ЗАРПЛАТА: %d с.\n\n", monthKey, salary)

	sb.WriteString("РАСХОДЫ ПО КАТЕГОРИЯМ:\n")
	for _, cat := range category.All() {
		spent := somoni(m.ByCategory[cat.Name])
		if spent == 0 && cat.Limit == 0 {
			continue
		}
		pct := 0.0
		if cat.Limit > 0 {
			pct = spent / float64(cat.Limit) * 100
		}
		fmt.Fprintf(&sb, "- %s: %.0f с. / %d с. лимит (%.0f%%)\n", cat.Label, spent, cat.Limit, pct)
	}
	fmt.Fprintf(&sb, "\nИТОГО РАСХОДЫ: %.0f с. | ДОХОДЫ: %.0f с.\n", somoni(m.Expense), somoni(m.Income))
	if m.CreditCharged != 0 || m.CreditRepaid != 0 {
		fmt.Fprintf(&sb, "КРЕДИТНАЯ КАРТА: потрачено %.0f с., погашено %.0f с.\n", somoni(m.CreditCharged), somoni(m.CreditRepaid))
	}
	fmt.Fprintf(&sb, "БАЛАНС НАКОПЛЕНИЙ (всё время): %.0f с.\n\n", somoni(savings))

	if len(goals) > 0 {
		sb.WriteString("КВАРТАЛЬНЫЕ ЦЕЛИ:\n")
		for _, g := range goals {
			status := "Активна"
			if g.Status == "done" {
				status = "Выполнена"
			}
			fmt.Fprintf(&sb, "- %s: цель %.0f с., квартал %s, статус: %s\n",
				g.Name, somoni(g.Target), g.Quarter, status)
			if g.Note != "" {
				fmt.Fprintf(&sb, "  %s\n", g.Note)
			}
		}
		fmt.Fprintf(&sb, "Текущий баланс накоплений (общий): %.0f с.\n", somoni(savings))
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

func (a *Advisor) callGemini(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(geminiRequest{
		Contents:         []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
		GenerationConfig: geminiGenConfig{MaxOutputTokens: 8192},
	})
	if err != nil {
		return "", err
	}

	url := geminiAPIURL + "?key=" + a.apiKey
	ctx, cancel := context.WithTimeout(ctx, 80*time.Second)
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

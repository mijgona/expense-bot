package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"expense-bot/internal/category"
	"expense-bot/internal/sheets"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	callbackCat    = "cat:"
	callbackReport = "rep:"
)

// Handler processes all incoming Telegram updates.
type Handler struct {
	bot     *tgbotapi.BotAPI
	sheets  *sheets.Client
	salary  int
	sheetID string
	states  *stateStore
}

func newHandler(bot *tgbotapi.BotAPI, sheetsClient *sheets.Client, salary int, sheetID string) *Handler {
	return &Handler{
		bot:     bot,
		sheets:  sheetsClient,
		salary:  salary,
		sheetID: sheetID,
		states:  newStateStore(),
	}
}

// HandleMessage routes incoming text messages.
func (h *Handler) HandleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	// If user is mid-conversation waiting to enter amount.
	if d := h.states.get(chatID); d != nil && d.Step == stepEnterAmount {
		h.handleAmountInput(chatID, text, d)
		return
	}

	// Try quick-input: "350" or "350 такси"
	if amt, desc, ok := parseQuickInput(text); ok {
		h.states.set(chatID, &dialog{Step: stepChooseCat, Amount: amt, Description: desc})
		preview := fmt.Sprintf("💸 *%s с.*", fmtNum(amt))
		if desc != "" {
			preview += " — " + desc
		}
		kb := categoryKeyboard()
		h.sendMarkdown(chatID, preview+"\n\n📂 Выбери категорию:", &kb)
		return
	}

	// Static menu buttons.
	switch text {
	case "/start":
		h.handleStart(msg)
	case "➕ Записать расход":
		h.states.set(chatID, &dialog{Step: stepChooseCat})
		kb := categoryKeyboard()
		h.sendMarkdown(chatID, "📂 *Выбери категорию:*", &kb)
	case "📊 Отчёт за месяц":
		h.handleReport(chatID, time.Now().Format("2006-01"))
	case "💰 Остаток":
		h.handleBalance(chatID)
	case "📋 Открыть таблицу":
		kb := sheetLinkKeyboard(h.sheetID)
		h.sendMarkdown(chatID, "👆 Нажми чтобы открыть:", &kb)
	}
}

// HandleCallback routes inline-button taps.
func (h *Handler) HandleCallback(query *tgbotapi.CallbackQuery) {
	h.bot.Request(tgbotapi.NewCallback(query.ID, ""))

	chatID := query.Message.Chat.ID
	msgID := query.Message.MessageID
	data := query.Data

	switch {
	case strings.HasPrefix(data, callbackCat):
		h.handleCategoryCallback(chatID, msgID, data[len(callbackCat):])
	case strings.HasPrefix(data, callbackReport):
		h.handleReportCallback(chatID, msgID, data[len(callbackReport):])
	}
}

// ── private handlers ──────────────────────────────────────────────────────────

func (h *Handler) handleStart(msg *tgbotapi.Message) {
	text := fmt.Sprintf(
		"👋 Привет, *%s*!\n\n"+
			"Я записываю расходы прямо в *Google Таблицу* 📊\n\n"+
			"Как добавить расход:\n"+
			"• Нажми *➕ Записать расход*\n"+
			"• Или напиши: `350 такси` или просто `350`\n\n"+
			"💼 Зарплата: *%s сомони*",
		msg.From.FirstName,
		fmtNum(float64(h.salary)),
	)
	kb := mainKeyboard()
	h.sendMarkdownWithReply(msg.Chat.ID, text, kb)
}

func (h *Handler) handleAmountInput(chatID int64, text string, d *dialog) {
	parts := strings.SplitN(text, " ", 2)
	amt, err := strconv.ParseFloat(strings.ReplaceAll(parts[0], ",", "."), 64)
	if err != nil || amt <= 0 {
		h.sendMarkdown(chatID, "❌ Введи число. Например: `350` или `350 обед`", nil)
		return
	}
	desc := ""
	if len(parts) > 1 {
		desc = parts[1]
	}
	catName := d.Category
	h.states.clear(chatID)
	h.recordAndConfirm(chatID, 0, catName, amt, desc)
}

func (h *Handler) handleCategoryCallback(chatID int64, msgID int, catName string) {
	if catName == "cancel" {
		h.states.clear(chatID)
		h.editText(chatID, msgID, "❌ Отменено", nil)
		return
	}

	d := h.states.get(chatID)
	if d == nil {
		return
	}

	// Quick-input path: amount already known.
	if d.Step == stepChooseCat && d.Amount > 0 {
		amt := d.Amount
		desc := d.Description
		h.states.clear(chatID)
		h.editText(chatID, msgID, "⏳ Записываю...", nil)
		h.recordAndConfirmEdit(chatID, msgID, catName, amt, desc)
		return
	}

	// Dialog path: ask for amount next.
	d.Step = stepEnterAmount
	d.Category = catName
	h.states.set(chatID, d)

	cat := category.FindByName(catName)
	limitStr := ""
	if cat != nil {
		limitStr = fmt.Sprintf(" (лимит: %s с.)", fmtNum(float64(cat.Limit)))
	}
	h.editText(chatID, msgID,
		fmt.Sprintf("*%s*%s\n\n💬 Введи *сумму* и описание:\nПример: `350 такси` или просто `350`",
			catName, limitStr),
		nil,
	)
}

func (h *Handler) handleReport(chatID int64, monthKey string) {
	h.sendMarkdown(chatID, "⏳ Загружаю...", nil)

	stats, err := h.sheets.GetMonthStats(monthKey)
	if err != nil {
		h.sendMarkdown(chatID, fmt.Sprintf("❌ Ошибка загрузки: `%v`", err), nil)
		return
	}

	now := time.Now()
	prevTime := now.AddDate(0, -1, 0)
	prevKey := prevTime.Format("2006-01")
	kb := reportNavKeyboard(monthKey, prevKey, ruMonth(prevTime.Month()))

	text := formatReport(stats, h.salary, monthKey)
	h.sendMarkdown(chatID, text, &kb)
}

func (h *Handler) handleReportCallback(chatID int64, msgID int, monthKey string) {
	stats, err := h.sheets.GetMonthStats(monthKey)
	if err != nil {
		h.editText(chatID, msgID, fmt.Sprintf("❌ Ошибка: `%v`", err), nil)
		return
	}
	text := formatReport(stats, h.salary, monthKey)
	h.editText(chatID, msgID, text, nil)
}

func (h *Handler) handleBalance(chatID int64) {
	h.sendMarkdown(chatID, "⏳ Считаю...", nil)

	stats, err := h.sheets.GetMonthStats(time.Now().Format("2006-01"))
	if err != nil {
		h.sendMarkdown(chatID, fmt.Sprintf("❌ Ошибка: `%v`", err), nil)
		return
	}
	kb := mainKeyboard()
	h.sendMarkdownWithReply(chatID, formatBalance(stats, h.salary), kb)
}

func (h *Handler) recordAndConfirm(chatID int64, msgID int, catName string, amt float64, desc string) {
	sheetTitle, err := h.sheets.AppendExpense(sheets.Expense{
		Category:    catName,
		Amount:      amt,
		Description: desc,
	})
	if err != nil {
		kb := mainKeyboard()
		h.sendMarkdownWithReply(chatID, fmt.Sprintf("❌ Ошибка записи: `%v`", err), kb)
		return
	}
	kb := mainKeyboard()
	h.sendMarkdownWithReply(chatID, buildConfirmText(catName, amt, desc, sheetTitle), kb)
}

func (h *Handler) recordAndConfirmEdit(chatID int64, msgID int, catName string, amt float64, desc string) {
	sheetTitle, err := h.sheets.AppendExpense(sheets.Expense{
		Category:    catName,
		Amount:      amt,
		Description: desc,
	})
	if err != nil {
		h.editText(chatID, msgID, fmt.Sprintf("❌ Ошибка: `%v`", err), nil)
		return
	}
	h.editText(chatID, msgID, buildConfirmText(catName, amt, desc, sheetTitle), nil)
}

func buildConfirmText(catName string, amt float64, desc, sheetTitle string) string {
	cat := category.FindByName(catName)
	label := catName
	if cat != nil {
		label = cat.Label
	}
	descStr := ""
	if desc != "" {
		descStr = "\n📝 _" + desc + "_"
	}
	return fmt.Sprintf("✅ *Записано!*\n\n%s: *%s с.*%s\n📅 %s\n📋 `%s`",
		label, fmtNum(amt), descStr,
		time.Now().Format("02.01.2006 15:04"),
		sheetTitle,
	)
}

// ── send helpers ──────────────────────────────────────────────────────────────

func (h *Handler) sendMarkdown(chatID int64, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if kb != nil {
		msg.ReplyMarkup = kb
	}
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send error: %v", err)
	}
}

func (h *Handler) sendMarkdownWithReply(chatID int64, text string, kb tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = kb
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send error: %v", err)
	}
}

func (h *Handler) editText(chatID int64, msgID int, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(chatID, msgID, text)
	edit.ParseMode = "Markdown"
	if kb != nil {
		edit.ReplyMarkup = kb
	}
	if _, err := h.bot.Send(edit); err != nil {
		log.Printf("edit error: %v", err)
	}
}

// ── parse helpers ─────────────────────────────────────────────────────────────

// parseQuickInput tries to parse "350" or "350 такси" from raw text.
func parseQuickInput(text string) (amt float64, desc string, ok bool) {
	parts := strings.SplitN(text, " ", 2)
	v, err := strconv.ParseFloat(strings.ReplaceAll(parts[0], ",", "."), 64)
	if err != nil || v <= 0 {
		return 0, "", false
	}
	if len(parts) > 1 {
		desc = parts[1]
	}
	return v, desc, true
}

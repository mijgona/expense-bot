package bot

import (
	"fmt"
	"time"

	"expense-bot/internal/category"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func mainKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("➕ Записать расход"),
			tgbotapi.NewKeyboardButton("💵 Записать приход"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Отчёт за месяц"),
			tgbotapi.NewKeyboardButton("💰 Остаток"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🏦 Накопления"),
			tgbotapi.NewKeyboardButton("🎯 Цели"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🤖 ИИ-отчёт"),
			tgbotapi.NewKeyboardButton("📋 Открыть таблицу"),
		),
	)
	kb.ResizeKeyboard = true
	return kb
}

// goalsKeyboard shows an inline button to add a new goal.
func goalsKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить цель", "goal:add"),
		),
	)
}

// quarterKeyboard shows the next 5 quarters as inline buttons (2 per row).
func quarterKeyboard() tgbotapi.InlineKeyboardMarkup {
	quarters := nextQuarters(5)
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(quarters); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(quarters[i], "goal:q:"+quarters[i]),
		}
		if i+1 < len(quarters) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(quarters[i+1], "goal:q:"+quarters[i+1]))
		}
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "goal:cancel"),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// nextQuarters returns n quarter labels starting from the current quarter, e.g. "Q2 2026".
func nextQuarters(n int) []string {
	now := time.Now()
	q := (int(now.Month())-1)/3 + 1
	year := now.Year()
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("Q%d %d", q, year))
		q++
		if q > 4 {
			q = 1
			year++
		}
	}
	return out
}

func categoryKeyboard() tgbotapi.InlineKeyboardMarkup {
	cats := category.All()
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(cats); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(cats[i].Label, callbackCat+cats[i].Name),
		}
		if i+1 < len(cats) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(cats[i+1].Label, callbackCat+cats[i+1].Name))
		}
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", callbackCat+"cancel"),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func savingsKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Пополнить", "sav:add"),
			tgbotapi.NewInlineKeyboardButtonData("➖ Снять", "sav:withdraw"),
		),
	)
}

func reportNavKeyboard(currentKey, prevKey, prevLabel string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ "+prevLabel, callbackReport+prevKey),
			tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", callbackReport+currentKey),
		),
	)
}

func sheetLinkKeyboard(spreadsheetID string) tgbotapi.InlineKeyboardMarkup {
	url := "https://docs.google.com/spreadsheets/d/" + spreadsheetID
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("📋 Открыть таблицу", url),
		),
	)
}

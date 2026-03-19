package bot

import (
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
			tgbotapi.NewKeyboardButton("📋 Открыть таблицу"),
		),
	)
	kb.ResizeKeyboard = true
	return kb
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

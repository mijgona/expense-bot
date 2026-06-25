# expense-bot

[![CI](https://github.com/mijgona/expense-bot/actions/workflows/ci.yml/badge.svg)](https://github.com/mijgona/expense-bot/actions/workflows/ci.yml)

🇷🇺 Русская версия: **[README.ru.md](./README.ru.md)**

A personal Telegram bot for expense tracking. It logs spending to a Google Sheet, shows monthly reports and remaining budget, manages savings and quarterly goals, and every 3 days sends an AI-generated financial review. Currency is the Tajik somoni (с.).

## Features

- **Quick input**: type `350` or `350 taxi` and the bot immediately asks you to pick a category
- 14 expense categories, each with a monthly limit
- **Income logging**: e.g. `5000 salary`
- **Monthly report** broken down by category, with limit-usage percentages
- **Balance**: how much is left and how much you can spend per day (carry-over from previous months included)
- 🏦 **Savings**: deposits and withdrawals, kept on a separate sheet
- 🎯 **Quarterly goals**: name, target amount, quarter, status
- 🤖 **AI report**: financial analysis from Gemini — on demand via a button and automatically every 3 days (delivered as a `.md` file)
- **Multi-user**: each user gets their own set of sheets in the spreadsheet
- **Authorization** by Telegram ID (optional)

## Spreadsheet layout

All sheets are created automatically. Each user gets their own set:

| Sheet | Columns | Purpose |
|-------|---------|---------|
| `<Telegram ID>` | Date \| Time \| Category \| Amount \| Description \| Month | Main log: expenses (negative amount) and income (positive, category «Приход») |
| `<Telegram ID>_savings` | Date \| Time \| Amount \| Description \| Month | Savings: deposits (+) and withdrawals (−) |
| `<Telegram ID>_goals` | Name \| Target amount (с.) \| Quarter \| Status \| Description | Quarterly goals |
| `Пользователи` | UserID \| Name \| Username \| Registration date | User registry (populated on /start) |

Example main-sheet row:

| Date | Time | Category | Amount | Description | Month |
|------|------|----------|--------|-------------|-------|
| 18.03.2026 | 14:32 | Еда | -350 | lunch | 2026-03 |

> **Note**: the UI and stored category/sheet names are in Russian — this is a personal bot. The table above translates them for readers; the actual values are the Russian originals.

## Categories

| Category | Limit (с./month) |
|----------|------------------|
| 🍽 Food/Groceries | 2 000 |
| 🏠 Rent/Utilities | 300 |
| 🚗 Transport | 1 000 |
| 👗 Clothing | 1 000 |
| 💊 Health | 500 |
| 📱 Connectivity | 200 |
| 🎭 Entertainment | 500 |
| 💰 Savings | 5 000 |
| 📚 Education | 200 |
| 🎒 Children's education | 3 000 |
| 💄 Personal | 1 000 |
| 💼 Business | 300 |
| 🔧 Other | 500 |
| 🤷 Not sure | 450 |

Limits and the category list are defined in `internal/category/category.go`.

## Setup

### 1. Telegram bot

1. Create a bot via [@BotFather](https://t.me/BotFather) and get a `BOT_TOKEN`

### 2. Google Sheets

1. Create a [Service Account](https://console.cloud.google.com/iam-admin/serviceaccounts) in Google Cloud
2. Enable the **Google Sheets API** for the project
3. Download the service-account JSON key (`credentials.json`)
4. Create a new Google Sheet and grant the service account **Editor** access
5. Copy the spreadsheet ID from the URL: `https://docs.google.com/spreadsheets/d/<SPREADSHEET_ID>/`

### 3. Gemini API (optional, for AI reports)

1. Get a key in [Google AI Studio](https://aistudio.google.com/apikey) → `GEMINI_API_KEY`

Without the key the bot works fully, except for the «🤖 ИИ-отчёт» button and the automatic reports.

### 4. Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `BOT_TOKEN` | yes | Bot token from @BotFather |
| `SPREADSHEET_ID` | yes | Google Sheet ID |
| `GOOGLE_CREDENTIALS_JSON` | yes (prod) | Contents of credentials.json on a single line |
| `GEMINI_API_KEY` | no | Gemini key for AI reports |
| `SALARY` | no | Monthly salary in somoni (default: 10000) |
| `ALLOWED_USER_IDS` | no | Comma-separated Telegram IDs allowed to use the bot |
| `PORT` | no | Health-check server port (default: 8080) |

## Running locally

```bash
# Install dependencies
go mod tidy

# Create config.json in the project root
cat > config.json <<EOF
{
  "bot_token": "...",
  "spreadsheet_id": "...",
  "salary": 10000,
  "gemini_api_key": "..."
}
EOF

# Put credentials.json in the project root

# Run
go run ./cmd/bot
```

Or via environment variables:

```bash
export BOT_TOKEN=...
export SPREADSHEET_ID=...
export GOOGLE_CREDENTIALS_JSON=$(cat credentials.json)
export GEMINI_API_KEY=...
go run ./cmd/bot
```

## Deploying to Render

1. Push the repository to GitHub
2. In Render: **New → Blueprint** and select the repository
3. In the service settings add the secret environment variables:
   - `BOT_TOKEN`
   - `SPREADSHEET_ID`
   - `GOOGLE_CREDENTIALS_JSON`
   - `SALARY` (your real value — set in the dashboard, not stored in the repo)
   - `GEMINI_API_KEY` (optional)

Region: Frankfurt (closest to Central Asia). The bot pings its own `/health` every 45 seconds so the free instance does not sleep.

## Project structure

```
cmd/bot/main.go               # entrypoint: config + sheets + bot + advisor + health server
internal/
  config/config.go            # config loading from env or file
  category/category.go        # 14 categories with limits
  sheets/client.go            # Google Sheets: expenses, savings, goals, users
  advisor/advisor.go          # AI advisor (Gemini): analysis every 3 days and on demand
  bot/
    bot.go                    # init and polling loop
    handler.go                # message and callback routing
    state.go                  # dialog state machine (sync.Mutex)
    keyboard.go               # keyboards
    formatter.go              # report formatting
```

## Stack

- **Go** 1.26
- **Telegram:** `github.com/go-telegram-bot-api/telegram-bot-api/v5`
- **Google Sheets:** `google.golang.org/api/sheets/v4`
- **AI:** Google Gemini 2.5 Flash (REST API)
- **Deploy:** Render (Web Service, Docker)

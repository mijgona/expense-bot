# expense-bot

Telegram Mini App для учёта расходов. Расходы и приходы, отчёты по месяцам с лимитами категорий, накопления, кредитная карта и квартальные цели. Раз в 3 дня приходит финансовый анализ от ИИ. Валюта — таджикский сомони (с.).

Приложение открывается кнопкой «Открыть» в чате с ботом. Бот только регистрирует пользователя по `/start` и присылает ИИ-отчёты, всё остальное делается в приложении.

## Возможности

- Расход: сумма, описание, категория (14 категорий с месячными лимитами)
- Приход
- Отчёт за любой месяц: перенос с прошлых месяцев, приход, расход, остаток, лимиты по категориям, список записей
- Главный экран: остаток, сколько дней осталось и сколько можно тратить в день
- 🏦 Накопления: пополнение и снятие (снять больше, чем накоплено, нельзя)
- 💳 Кредитная карта: покупки по категориям и погашения (погасить больше долга нельзя)
- 🎯 Квартальные цели с прогрессом от накоплений
- 🤖 ИИ-отчёт от Gemini: по кнопке в приложении и автоматически каждые 3 дня файлом в чат
- Несколько пользователей: у каждого свои данные; пользователя определяет подпись Telegram
- Ограничение доступа по Telegram ID (необязательно)
- Светлая и тёмная тема берутся из Telegram

## Архитектура

```
Telegram ──► webapp/ (Vite + React, Render Static Site)
                │  /api/*  Authorization: tma <initData>
                ▼
             Go API + бот + ИИ-советник (Render Web Service, Docker)
                │
                ▼
             Firestore (europe-west3)
```

- Деньги хранятся в дирамах (1 с. = 100), всегда положительными числами; направление задаёт тип записи.
- Месяц считается по времени Душанбе.
- Итоги месяца обновляются в одной транзакции с записью.

Подробнее — в `specs/004-telegram-mini-app/` (spec, plan, data-model, contracts, quickstart).

## Категории

Лимиты и список категорий задаются в `internal/category/category.go`. Приложение получает их с сервера.

## Настройка

### 1. Telegram

Создай бота через [@BotFather](https://t.me/BotFather) и получи `BOT_TOKEN`. Кнопку меню «Открыть» бот поставит сам при запуске, если задан `WEBAPP_URL`.

### 2. Firestore

1. Firebase console: создай проект или подключи Firebase к GCP-проекту сервисного аккаунта.
2. Firestore Database → **Native mode**, регион **europe-west3**. Регион потом не меняется.
3. Опубликуй правила из `firestore.rules` (они запрещают любой доступ с клиента).
4. В IAM дай сервисному аккаунту роль **Cloud Datastore User**.
5. Создай индекс из `firestore.indexes.json`: `firebase deploy --only firestore:indexes`.

### 3. Gemini (необязательно)

Ключ в [Google AI Studio](https://aistudio.google.com/apikey) → `GEMINI_API_KEY`. Без ключа работает всё, кроме ИИ-отчётов.

### 4. Переменные окружения

| Переменная | Обязательная | Описание |
|------------|-------------|----------|
| `BOT_TOKEN` | да | Токен бота; им же проверяется подпись Telegram |
| `GOOGLE_CREDENTIALS_JSON` | да (прод) | Содержимое credentials.json сервисного аккаунта |
| `FIRESTORE_PROJECT_ID` | нет | По умолчанию берётся `project_id` из credentials |
| `WEBAPP_URL` | да (прод) | Адрес Mini App (CORS, кнопка в боте) |
| `GEMINI_API_KEY` | нет | ИИ-отчёты |
| `SALARY` | нет | Зарплата в сомони (по умолчанию 16000) |
| `ALLOWED_USER_IDS` | нет | Telegram ID через запятую |
| `DEV_USER_ID` | нет | Только для локальной разработки: работать без Telegram от имени этого ID |
| `SPREADSHEET_ID` | только миграция | Старая Google Таблица |

Для фронтенда: `VITE_API_URL` — адрес API, вшивается при сборке.

## Запуск локально

```bash
# API + бот (config.json с bot_token + credentials.json в корне)
export DEV_USER_ID=<твой telegram id> FIRESTORE_PROJECT_ID=<dev-проект>
go run ./cmd/bot

# Mini App
cd webapp && npm ci && npm run dev   # http://localhost:5173
```

Проверки перед коммитом:

```bash
go build ./... && go vet ./... && go test ./...
cd webapp && npm run typecheck && npm run build
```

## Миграция из Google Таблицы

```bash
export GOOGLE_CREDENTIALS_JSON="$(cat credentials.json)" SPREADSHEET_ID=... FIRESTORE_PROJECT_ID=...
go run ./cmd/migrate -dry-run   # ничего не пишет
go run ./cmd/migrate            # импорт + пересчёт итогов + сверка; при расхождениях exit 1
```

Миграцию можно запускать повторно, данные не задвоятся. Порядок переключения прода описан в `specs/004-telegram-mini-app/quickstart.md` §5.

## Деплой на Render

`render.yaml` создаёт два сервиса:
- **expense-bot**: API, бот и ИИ-советник;
- **expense-bot-webapp**: Mini App.

В Dashboard задай секреты (`BOT_TOKEN`, `GOOGLE_CREDENTIALS_JSON`, `WEBAPP_URL`, при необходимости `GEMINI_API_KEY`, `ALLOWED_USER_IDS`, `FIRESTORE_PROJECT_ID`), а у статического сайта — `VITE_API_URL`. Сервис сам пингует `/health` каждые 45 секунд, чтобы бесплатный инстанс не засыпал.

## Стек

- **Go** 1.26: `net/http`, `cloud.google.com/go/firestore`; Telegram Bot API и Gemini вызываются через raw HTTP
- **Mini App:** Vite, React 19, TypeScript, react-markdown
- **ИИ:** Google Gemini 2.5 Flash
- **Deploy:** Render (Docker web service + static site), регион Frankfurt

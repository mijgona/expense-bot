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
- 🎯 Квартальные цели с прогрессом от накоплений: можно редактировать, отмечать выполненными и удалять
- 📜 История всех записей за всё время: фильтры по типу, категории и месяцу, поиск по описанию
- ✏️ Редактирование и удаление любой записи. Итоги месяцев, перенос остатка, накопления и долг пересчитываются автоматически; накопления и долг не могут уйти в минус ни на одну дату
- 👤 Профиль: своё имя и зарплата
- 💵 Зарплата двумя частями: аванс 15-го и остаток в последний день месяца. Запись в один тап, напоминание от бота в 10:00 в день выплаты, бюджет «до следующей выплаты»
- 🏷 Свои категории (Профиль → Категории): добавлять, переименовывать (новое имя видно во всей истории), скрывать, менять порядок и лимиты
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

## Обновление до версии с историей и редактированием (фича 005)

Перед деплоем, один раз:

```bash
firebase deploy --only firestore:indexes --project precise-plane-340808   # дождаться статуса «Enabled»
go run ./cmd/migrate -backfill-005    # проставить дату записи и версию у старых записей
go run ./cmd/migrate -rebuild         # проверка: 0 mismatches
```

Без этих шагов история покажет не все записи.

## Обновление до своих категорий (фича 006)

Бэкенд и Mini App обновляются вместе (контракт API 2.0.0):

```bash
go run ./cmd/migrate -convert-006 -dry-run   # что будет создано и переписано
go run ./cmd/migrate -convert-006            # переход на ID категорий, проверка: 0 mismatches
# деплой
go run ./cmd/migrate -convert-006            # подхватить записи, сделанные старой версией
go run ./cmd/migrate -rebuild                # 0 mismatches
```

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

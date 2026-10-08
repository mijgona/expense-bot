# ── build stage ───────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Cache module downloads separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o expense-bot ./cmd/bot

# ── final stage ───────────────────────────────────────────────────────────────
FROM alpine:3.19

# ca-certificates needed for HTTPS calls to Google / Telegram APIs.
# Time zone data is embedded in the binary (time/tzdata).
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/expense-bot .

# On Render: all config comes from env vars (BOT_TOKEN, GOOGLE_CREDENTIALS_JSON, WEBAPP_URL, etc.)
# Locally: mount config.json + credentials.json into /app/
CMD ["./expense-bot"]

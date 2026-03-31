package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"expense-bot/internal/bot"
	"expense-bot/internal/config"
	"expense-bot/internal/sheets"
)

func main() {
	cfg := loadConfig()
	sheetsClient := loadSheets(cfg.SpreadsheetID)

	application, err := bot.New(cfg, sheetsClient)
	if err != nil {
		log.Fatalf("bot init: %v", err)
	}

	// Render web services require an HTTP listener on $PORT.
	go func() {
		port := envOr("PORT", "8080")
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
		log.Printf("health: listening on :%s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("health server: %v", err)
		}
	}()

	go selfPing()

	application.Run()
}

// loadConfig prefers environment variables (Render / Docker),
// and falls back to config.json for local development.
func loadConfig() *config.Config {
	if os.Getenv("BOT_TOKEN") != "" {
		log.Println("config: loading from environment variables")
		cfg, err := config.LoadFromEnv()
		if err != nil {
			log.Fatalf("config (env): %v", err)
		}
		return cfg
	}

	path := envOr("CONFIG_PATH", "config.json")
	log.Printf("config: loading from file %q", path)
	cfg, err := config.Load(path)
	if err != nil {
		log.Fatalf("config (file): %v", err)
	}
	return cfg
}

// loadSheets prefers GOOGLE_CREDENTIALS_JSON env var (Render / Docker),
// and falls back to credentials.json for local development.
func loadSheets(spreadsheetID string) *sheets.Client {
	if raw := os.Getenv("GOOGLE_CREDENTIALS_JSON"); raw != "" {
		log.Println("sheets: loading credentials from GOOGLE_CREDENTIALS_JSON")
		client, err := sheets.NewFromJSON([]byte(raw), spreadsheetID)
		if err != nil {
			log.Fatalf("sheets (env): %v", err)
		}
		return client
	}

	path := envOr("CREDENTIALS_PATH", "credentials.json")
	log.Printf("sheets: loading credentials from file %q", path)
	client, err := sheets.New(path, spreadsheetID)
	if err != nil {
		log.Fatalf("sheets (file): %v", err)
	}
	return client
}

// selfPing keeps the Render free-tier service awake by pinging /health every 10 minutes.
// It uses RENDER_EXTERNAL_URL which Render sets automatically.
func selfPing() {
	url := os.Getenv("RENDER_EXTERNAL_URL")
	if url == "" {
		return // not running on Render
	}
	url += "/health"
	for range time.Tick(45 * time.Second) {
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("self-ping error: %v", err)
			continue
		}
		resp.Body.Close()
		log.Printf("self-ping: %s %s", resp.Status, url)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

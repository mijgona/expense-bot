package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"expense-bot/internal/advisor"
	"expense-bot/internal/api"
	"expense-bot/internal/config"
	"expense-bot/internal/store/firestore"
	"expense-bot/internal/telegram"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := loadConfig()
	st := loadStore(ctx, cfg)
	defer st.Close()

	tg := telegram.NewClient(cfg.BotToken)
	adv := advisor.New(st, cfg.GeminiAPIKey, cfg.Salary,
		func(ctx context.Context, userID int64, filename string, md []byte) error {
			return tg.SendDocument(ctx, userID, filename, md, "🤖 Финансовый отчёт", cfg.WebAppURL)
		})

	// Render web services require an HTTP listener on $PORT: /health + the Mini App API.
	port := envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           api.New(st, cfg, adv).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("api: listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("api server: %v", err)
		}
	}()

	go selfPing()
	go adv.Start(ctx)
	go telegram.NewBot(tg, st, cfg).Start(ctx)

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
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

// loadStore prefers GOOGLE_CREDENTIALS_JSON env var (Render / Docker),
// and falls back to credentials.json for local development.
func loadStore(ctx context.Context, cfg *config.Config) *firestore.Store {
	creds := []byte(os.Getenv("GOOGLE_CREDENTIALS_JSON"))
	if len(creds) > 0 {
		log.Println("store: loading credentials from GOOGLE_CREDENTIALS_JSON")
	} else {
		path := envOr("CREDENTIALS_PATH", "credentials.json")
		log.Printf("store: loading credentials from file %q", path)
		var err error
		if creds, err = os.ReadFile(path); err != nil {
			log.Fatalf("store: read credentials: %v", err)
		}
	}
	st, err := firestore.New(ctx, creds, cfg.FirestoreProjectID)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	return st
}

// selfPing keeps the Render free-tier service awake by pinging /health every 45 seconds.
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
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

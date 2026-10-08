// Package api is the JSON API used by the Telegram Mini App (contracts/api.openapi.yaml).
// Every /api/* route is wrapped by the single auth middleware (constitution IV).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"

	"expense-bot/internal/config"
	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

// Advisor generates the AI report. It may be nil or disabled.
type Advisor interface {
	Enabled() bool
	Generate(ctx context.Context, userID int64) (string, error)
}

// Server holds API dependencies.
type Server struct {
	store   store.Store
	cfg     *config.Config
	advisor Advisor

	advisorLocks sync.Map // userID → *sync.Mutex
}

// New creates the API server. adv may be nil.
func New(st store.Store, cfg *config.Config, adv Advisor) *Server {
	if cfg.DevUserID != 0 {
		log.Printf("api: DEV_USER_ID=%d — initData verification DISABLED", cfg.DevUserID)
	}
	return &Server{store: st, cfg: cfg, advisor: adv}
}

// Handler returns the HTTP handler with /health and all /api routes.
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("POST /api/session", s.handleSession)
	api.HandleFunc("GET /api/summary", s.handleSummary)
	api.HandleFunc("GET /api/transactions", s.handleListTransactions)
	api.HandleFunc("POST /api/transactions", s.handleAddTransaction)
	api.HandleFunc("GET /api/goals", s.handleListGoals)
	api.HandleFunc("POST /api/goals", s.handleAddGoal)
	api.HandleFunc("POST /api/advisor/report", s.handleAdvisorReport)

	root := http.NewServeMux()
	root.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	root.Handle("/api/", s.auth(api))
	return s.cors(root)
}

// cors allows the Mini App origin only; preflight is answered before auth.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && origin == s.cfg.WebAppURL {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── helpers ──────────────────────────────────────────────────────────────────

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message, field string) {
	var b errorBody
	b.Error.Code, b.Error.Message, b.Error.Field = code, message, field
	writeJSON(w, status, b)
}

func validation(w http.ResponseWriter, field, message string) {
	writeError(w, http.StatusBadRequest, "validation", message, field)
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("Некорректный запрос: %v", err)
	}
	return nil
}

// writeStoreError maps store errors to API errors (422 for business rules, 500 otherwise).
func writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	var le *store.LimitError
	switch {
	case errors.As(err, &le) && errors.Is(err, store.ErrInsufficientSavings):
		writeError(w, http.StatusUnprocessableEntity, "insufficient_savings",
			"Недостаточно накоплений: доступно "+ledger.FormatSomoni(le.Available), "amount")
	case errors.As(err, &le) && errors.Is(err, store.ErrExceedsDebt):
		writeError(w, http.StatusUnprocessableEntity, "exceeds_debt",
			"Сумма больше долга по карте: "+ledger.FormatSomoni(le.Available), "amount")
	default:
		log.Printf("api: %s %s: %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusInternalServerError, "internal", "Ошибка сервера, попробуйте ещё раз", "")
	}
}

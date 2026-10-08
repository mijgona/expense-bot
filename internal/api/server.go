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
	"time"

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
	api.HandleFunc("PATCH /api/transactions/{id}", s.handlePatchTransaction)
	api.HandleFunc("DELETE /api/transactions/{id}", s.handleDeleteTransaction)
	api.HandleFunc("GET /api/months", s.handleListMonths)
	api.HandleFunc("GET /api/profile", s.handleGetProfile)
	api.HandleFunc("PATCH /api/profile", s.handlePatchProfile)
	api.HandleFunc("GET /api/goals", s.handleListGoals)
	api.HandleFunc("POST /api/goals", s.handleAddGoal)
	api.HandleFunc("PATCH /api/goals/{id}", s.handlePatchGoal)
	api.HandleFunc("DELETE /api/goals/{id}", s.handleDeleteGoal)
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
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
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

type errorDetail struct {
	Code    string     `json:"code"`
	Message string     `json:"message"`
	Field   string     `json:"field,omitempty"`
	At      *time.Time `json:"at,omitempty"`
	Balance *int64     `json:"balance,omitempty"`
	Current any        `json:"current,omitempty"`
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message, field string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, Field: field}})
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

// writeStoreError maps store errors to API errors: 404 not_found, 409 conflict (+current),
// 422 balance rule (+at, balance), 500 otherwise.
func writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	var le *store.LimitError
	var ce *store.ConflictError
	switch {
	case errors.As(err, &le):
		code, what := "insufficient_savings", "накопления ушли бы в"
		if errors.Is(err, store.ErrExceedsDebt) {
			code, what = "exceeds_debt", "долг по карте стал бы"
		}
		d := errorDetail{Code: code, Field: "amount"}
		if le.At.IsZero() {
			d.Message = "Недостаточно средств: доступно " + ledger.FormatSomoni(le.Available)
		} else {
			at, bal := le.At.UTC(), le.Balance
			d.At, d.Balance = &at, &bal
			d.Message = fmt.Sprintf("%s: %s %s", le.At.In(ledger.Location).Format("02.01.2006"), what, ledger.FormatSomoni(le.Balance))
		}
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{Error: d})
	case errors.As(err, &ce):
		var cur any = ce.Current
		switch c := ce.Current.(type) {
		case store.Transaction:
			cur = toDTO(c)
		case store.Goal:
			cur = goalToDTO(c, 0)
		}
		writeJSON(w, http.StatusConflict, errorBody{Error: errorDetail{
			Code: "conflict", Message: "Запись изменили на другом устройстве", Current: cur}})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Запись не найдена", "")
	default:
		log.Printf("api: %s %s: %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusInternalServerError, "internal", "Ошибка сервера, попробуйте ещё раз", "")
	}
}

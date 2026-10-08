package api

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"expense-bot/internal/category"
	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type transactionDTO struct {
	ID        string      `json:"id"`
	Kind      ledger.Kind `json:"kind"`
	Category  *string     `json:"category"`
	Amount    int64       `json:"amount"`
	Note      string      `json:"note"`
	Month     string      `json:"month"`
	CreatedAt time.Time   `json:"createdAt"`
}

func toDTO(t store.Transaction) transactionDTO {
	d := transactionDTO{ID: t.ID, Kind: t.Kind, Amount: t.Amount, Note: t.Note, Month: t.Month, CreatedAt: t.CreatedAt}
	if t.Category != "" {
		c := t.Category
		d.Category = &c
	}
	return d
}

type newTransaction struct {
	ClientID string `json:"clientId"`
	Kind     string `json:"kind"`
	Amount   int64  `json:"amount"`
	Category string `json:"category"`
	Note     string `json:"note"`
}

// handleAddTransaction records any ledger entry; idempotent by clientId (FR-010).
func (s *Server) handleAddTransaction(w http.ResponseWriter, r *http.Request) {
	var in newTransaction
	if err := decodeJSON(r, &in); err != nil {
		validation(w, "", err.Error())
		return
	}
	if !uuidRe.MatchString(in.ClientID) {
		validation(w, "clientId", "Некорректный идентификатор формы")
		return
	}
	kind, err := ledger.ParseKind(in.Kind)
	if err != nil {
		validation(w, "kind", err.Error())
		return
	}
	if err := ledger.ValidateAmount(in.Amount); err != nil {
		validation(w, "amount", err.Error())
		return
	}
	note, err := ledger.ValidateNote(in.Note)
	if err != nil {
		validation(w, "note", err.Error())
		return
	}
	switch {
	case kind.NeedsCategory() && category.FindByName(in.Category) == nil:
		validation(w, "category", "Выберите категорию")
		return
	case !kind.NeedsCategory() && in.Category != "":
		validation(w, "category", "Для этой записи категория не нужна")
		return
	}

	now := ledger.Now()
	uid := userFrom(r.Context()).ID
	stored, created, err := s.store.AddTransaction(r.Context(), uid, store.Transaction{
		ID:        in.ClientID,
		Kind:      kind,
		Category:  in.Category,
		Amount:    in.Amount,
		Note:      note,
		Month:     ledger.MonthKey(now),
		Source:    "app",
		CreatedAt: now.UTC(),
	})
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	sum, err := s.buildSummary(r.Context(), uid, ledger.MonthKey(now))
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"transaction": toDTO(stored), "summary": sum})
}

func (s *Server) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	month, msg := parseReportMonth(r, true)
	if msg != "" {
		validation(w, "month", msg)
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			validation(w, "limit", "limit должен быть от 1 до 200")
			return
		}
		limit = n
	}
	txs, err := s.store.ListTransactions(r.Context(), userFrom(r.Context()).ID, month, limit)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	items := make([]transactionDTO, 0, len(txs))
	for _, t := range txs {
		items = append(items, toDTO(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

package api

import (
	"net/http"

	"expense-bot/internal/ledger"
)

type monthDTO struct {
	Month         string `json:"month"`
	Income        int64  `json:"income"`
	Expense       int64  `json:"expense"`
	SavingsNet    int64  `json:"savingsNet"`
	CreditCharged int64  `json:"creditCharged"`
	CreditRepaid  int64  `json:"creditRepaid"`
	CashNet       int64  `json:"cashNet"`
}

// handleListMonths returns stored month aggregates for History headers (FR-002).
func (s *Server) handleListMonths(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	for field, v := range map[string]string{"from": from, "to": to} {
		if v != "" {
			if _, err := ledger.ParseMonth(v); err != nil {
				validation(w, field, err.Error())
				return
			}
		}
	}
	ms, err := s.store.ListMonths(r.Context(), userFrom(r.Context()).ID, from, to)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	items := make([]monthDTO, 0, len(ms))
	for _, m := range ms {
		items = append(items, monthDTO{m.Month, m.Income, m.Expense, m.SavingsNet, m.CreditCharged, m.CreditRepaid, m.CashNet})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

package api

import (
	"net/http"
	"time"

	"expense-bot/internal/ledger"
	"expense-bot/internal/payroll"
	"expense-bot/internal/store"
)

// prevMonthGraceDays: the previous month's last-day payment is still offered during the
// first days of a new month (research R5).
const prevMonthGraceDays = 5

type payoutDTO struct {
	Month         string  `json:"month"`
	Kind          string  `json:"kind"`
	Amount        int64   `json:"amount"`
	Payday        string  `json:"payday"`
	Note          string  `json:"note"`
	Status        string  `json:"status"` // upcoming | due | recorded | dismissed
	TransactionID *string `json:"transactionId"`
}

// payoutStatus resolves one expected payment's status.
func (s *Server) payoutStatus(r *http.Request, uid int64, month string, p payroll.Payment, states map[string]store.Payout) (payoutDTO, error) {
	d := payoutDTO{Month: month, Kind: string(p.Kind), Amount: p.Amount, Payday: p.Payday.Format("2006-01-02"), Note: p.Note}
	id := payroll.TxID(month, p.Kind)
	_, err := s.store.GetTransaction(r.Context(), uid, id)
	switch {
	case err == nil:
		d.Status, d.TransactionID = "recorded", &id
	case err != store.ErrNotFound:
		return d, err
	case states[string(p.Kind)].DismissedAt != nil:
		d.Status = "dismissed"
	case !ledger.Now().Before(p.Payday):
		d.Status = "due"
	default:
		d.Status = "upcoming"
	}
	return d, nil
}

func (s *Server) monthPayouts(r *http.Request, u store.User, month string, only func(payroll.Payment) bool) ([]payoutDTO, error) {
	states, err := s.store.GetPayouts(r.Context(), u.ID, month)
	if err != nil {
		return nil, err
	}
	var out []payoutDTO
	for _, p := range payroll.ExpectedFor(u, s.cfg.Salary, month) {
		if only != nil && !only(p) {
			continue
		}
		d, err := s.payoutStatus(r, u.ID, month, p, states)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// handleListPayouts returns the month's expected payments with status (FR-005).
func (s *Server) handleListPayouts(w http.ResponseWriter, r *http.Request) {
	now := ledger.Now()
	cur := ledger.MonthKey(now)
	month := r.URL.Query().Get("month")
	if month == "" {
		month = cur
	} else if _, err := ledger.ParseMonth(month); err != nil {
		validation(w, "month", err.Error())
		return
	}
	u, err := s.store.GetUser(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	items := []payoutDTO{}
	if month == cur && now.Day() <= prevMonthGraceDays {
		prev, err := s.monthPayouts(r, u, ledger.PrevMonth(cur), func(p payroll.Payment) bool { return p.Kind != payroll.KindAdvance })
		if err != nil {
			writeStoreError(w, r, err)
			return
		}
		items = append(items, prev...)
	}
	list, err := s.monthPayouts(r, u, month, nil)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": append(items, list...)})
}

// expectedPayment validates {month}/{kind} against the user's schedule.
func (s *Server) expectedPayment(w http.ResponseWriter, r *http.Request) (store.User, string, payroll.Payment, bool) {
	month := r.PathValue("month")
	if _, err := ledger.ParseMonth(month); err != nil {
		validation(w, "month", err.Error())
		return store.User{}, "", payroll.Payment{}, false
	}
	kind, err := payroll.ParseKind(r.PathValue("kind"))
	if err != nil {
		validation(w, "kind", err.Error())
		return store.User{}, "", payroll.Payment{}, false
	}
	u, err := s.store.GetUser(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeStoreError(w, r, err)
		return store.User{}, "", payroll.Payment{}, false
	}
	for _, p := range payroll.ExpectedFor(u, s.cfg.Salary, month) {
		if p.Kind == kind {
			return u, month, p, true
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "Такой выплаты нет в вашем графике", "kind")
	return store.User{}, "", payroll.Payment{}, false
}

type recordPayout struct {
	Amount int64  `json:"amount"`
	Date   string `json:"date"`
}

// handleRecordPayout records a payment as income; idempotent by its deterministic ID (FR-007).
func (s *Server) handleRecordPayout(w http.ResponseWriter, r *http.Request) {
	var in recordPayout
	if err := decodeJSON(r, &in); err != nil {
		validation(w, "", err.Error())
		return
	}
	u, month, p, ok := s.expectedPayment(w, r)
	if !ok {
		return
	}
	if err := ledger.ValidateAmount(in.Amount); err != nil {
		validation(w, "amount", err.Error())
		return
	}
	day := p.Payday
	if in.Date != "" {
		d, msg := parseDate(in.Date)
		if msg != "" {
			validation(w, "date", msg)
			return
		}
		day = d
	} else if day.After(ledger.Now()) {
		validation(w, "date", "Дата не может быть в будущем")
		return
	}
	now := ledger.Now()
	occurred := time.Date(day.Year(), day.Month(), day.Day(), now.Hour(), now.Minute(), now.Second(), 0, ledger.Location)

	stored, created, err := s.store.AddTransaction(r.Context(), u.ID, store.Transaction{
		ID:         payroll.TxID(month, p.Kind),
		Kind:       ledger.KindIncome,
		Amount:     in.Amount,
		Note:       p.Note,
		Month:      ledger.MonthKey(occurred),
		Source:     "payout",
		OccurredAt: occurred.UTC(),
		CreatedAt:  now.UTC(),
		Version:    1,
	})
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	sum, err := s.buildSummary(r.Context(), u.ID, ledger.MonthKey(now))
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

// handleDismissPayout hides the offer for the month (FR-008).
func (s *Server) handleDismissPayout(w http.ResponseWriter, r *http.Request) {
	u, month, p, ok := s.expectedPayment(w, r)
	if !ok {
		return
	}
	if err := s.store.DismissPayout(r.Context(), u.ID, month, string(p.Kind)); err != nil {
		writeStoreError(w, r, err)
		return
	}
	states, err := s.store.GetPayouts(r.Context(), u.ID, month)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	d, err := s.payoutStatus(r, u.ID, month, p, states)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

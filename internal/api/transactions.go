package api

import (
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type transactionDTO struct {
	ID         string      `json:"id"`
	Kind       ledger.Kind `json:"kind"`
	Category   *string     `json:"category"`
	Amount     int64       `json:"amount"`
	Note       string      `json:"note"`
	GoalID     *string     `json:"goalId"`
	Month      string      `json:"month"`
	OccurredAt time.Time   `json:"occurredAt"`
	CreatedAt  time.Time   `json:"createdAt"`
	EditedAt   *time.Time  `json:"editedAt"`
	Version    int64       `json:"version"`
}

func toDTO(t store.Transaction) transactionDTO {
	d := transactionDTO{ID: t.ID, Kind: t.Kind, Amount: t.Amount, Note: t.Note, Month: t.Month,
		OccurredAt: t.When(), CreatedAt: t.CreatedAt, EditedAt: t.EditedAt, Version: t.Version}
	if d.Version == 0 {
		d.Version = 1
	}
	if t.Category != "" {
		c := t.Category
		d.Category = &c
	}
	if t.GoalID != "" {
		g := t.GoalID
		d.GoalID = &g
	}
	return d
}

// activeGoal reports whether id is one of the user's active goals (a deposit can be linked to it).
func (s *Server) activeGoal(r *http.Request, id string) (bool, error) {
	goals, err := s.store.ListGoals(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		return false, err
	}
	for _, g := range goals {
		if g.ID == id {
			return g.Status != "done", nil
		}
	}
	return false, nil
}

// checkGoal validates a deposit's goal link; it returns a validation message or "".
func (s *Server) checkGoal(r *http.Request, kind ledger.Kind, goalID string) (string, error) {
	if goalID == "" {
		return "", nil
	}
	if kind != ledger.KindSavingsDeposit {
		return "Цель указывается только для пополнения накоплений", nil
	}
	ok, err := s.activeGoal(r, goalID)
	if err != nil || ok {
		return "", err
	}
	return "Выберите активную цель из списка", nil
}

// parseDate validates "YYYY-MM-DD" (Dushanbe), not after today.
func parseDate(s string) (time.Time, string) {
	d, err := time.ParseInLocation("2006-01-02", s, ledger.Location)
	if err != nil {
		return time.Time{}, "Дата должна быть в формате ГГГГ-ММ-ДД"
	}
	now := ledger.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, ledger.Location)
	if d.After(today) {
		return time.Time{}, "Дата не может быть в будущем"
	}
	return d, ""
}

type newTransaction struct {
	ClientID string `json:"clientId"`
	Kind     string `json:"kind"`
	Amount   int64  `json:"amount"`
	Category string `json:"category"`
	Note     string `json:"note"`
	Date     string `json:"date"`
	GoalID   string `json:"goalId"`
}

// handleAddTransaction records any ledger entry; idempotent by clientId.
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
	if !kind.NeedsCategory() && in.Category != "" {
		validation(w, "category", "Для этой записи категория не нужна")
		return
	}
	if kind.NeedsCategory() {
		ok, err := s.usableCategory(r, in.Category)
		if err != nil {
			writeStoreError(w, r, err)
			return
		}
		if !ok {
			validation(w, "category", "Выберите категорию из списка")
			return
		}
	}

	if msg, err := s.checkGoal(r, kind, in.GoalID); err != nil {
		writeStoreError(w, r, err)
		return
	} else if msg != "" {
		validation(w, "goalId", msg)
		return
	}

	now := ledger.Now()
	occurred := now
	if in.Date != "" {
		d, msg := parseDate(in.Date)
		if msg != "" {
			validation(w, "date", msg)
			return
		}
		occurred = time.Date(d.Year(), d.Month(), d.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), ledger.Location)
	}

	uid := userFrom(r.Context()).ID
	stored, created, err := s.store.AddTransaction(r.Context(), uid, store.Transaction{
		ID:         in.ClientID,
		Kind:       kind,
		Category:   in.Category,
		Amount:     in.Amount,
		Note:       note,
		GoalID:     in.GoalID,
		Month:      ledger.MonthKey(occurred),
		Source:     "app",
		OccurredAt: occurred.UTC(),
		CreatedAt:  now.UTC(),
		Version:    1,
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

var groups = map[string][]ledger.Kind{
	"expense": {ledger.KindExpense},
	"income":  {ledger.KindIncome},
	"savings": {ledger.KindSavingsDeposit, ledger.KindSavingsWithdrawal},
	"credit":  {ledger.KindCreditPurchase, ledger.KindCreditRepayment},
}

// handleListTransactions is the History endpoint (filters + cursor). `?month=` alone keeps
// the 004 behaviour for the report screen.
func (s *Server) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.HistoryFilter{Category: q.Get("category"), Q: strings.TrimSpace(q.Get("q")), Cursor: q.Get("cursor"), Limit: 50}

	if g := q.Get("group"); g != "" {
		kinds, ok := groups[g]
		if !ok {
			validation(w, "group", "Неизвестный тип записей")
			return
		}
		f.Kinds = kinds
	}
	if m := q.Get("month"); m != "" {
		month, msg := parseReportMonth(r, false)
		if msg != "" {
			validation(w, "month", msg)
			return
		}
		f.Month = month
	}
	if utf8.RuneCountInString(f.Q) > 100 {
		validation(w, "q", "Слишком длинный поиск")
		return
	}
	maxLimit := 100
	if f.Month != "" && q.Get("group") == "" && f.Category == "" && f.Q == "" && f.Cursor == "" {
		maxLimit = 200 // 004 report list compatibility
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			validation(w, "limit", "limit должен быть от 1 до "+strconv.Itoa(maxLimit))
			return
		}
		f.Limit = n
	}

	uid := userFrom(r.Context()).ID
	txs, next, err := s.store.QueryTransactions(r.Context(), uid, f)
	if err != nil {
		if strings.Contains(err.Error(), "invalid cursor") {
			validation(w, "cursor", "Некорректный курсор")
			return
		}
		writeStoreError(w, r, err)
		return
	}
	items := make([]transactionDTO, 0, len(txs))
	var pageSum int64
	for _, t := range txs {
		items = append(items, toDTO(t))
		pageSum += t.Amount
	}

	total, exact := pageSum, false
	if f.Q == "" {
		// The exact total is a convenience: if the aggregation fails (e.g. a missing index),
		// fall back to the loaded page instead of failing the whole History request.
		if sum, err := s.store.SumTransactions(r.Context(), uid, f); err != nil {
			log.Printf("api: history total (falling back to page sum): %v", err)
		} else {
			total, exact = sum, true
		}
	}
	var nextPtr *string
	if next != "" {
		nextPtr = &next
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": nextPtr, "total": total, "totalExact": exact})
}

type transactionPatch struct {
	Version   int64   `json:"version"`
	RequestID string  `json:"requestId"`
	Amount    *int64  `json:"amount"`
	Category  *string `json:"category"`
	Note      *string `json:"note"`
	Date      *string `json:"date"`
	GoalID    *string `json:"goalId"` // "" unlinks
}

// handlePatchTransaction edits a record (kind is immutable).
func (s *Server) handlePatchTransaction(w http.ResponseWriter, r *http.Request) {
	var in transactionPatch
	if err := decodeJSON(r, &in); err != nil {
		validation(w, "", err.Error())
		return
	}
	if in.Version < 1 {
		validation(w, "version", "Не указана версия записи")
		return
	}
	if !uuidRe.MatchString(in.RequestID) {
		validation(w, "requestId", "Некорректный идентификатор запроса")
		return
	}
	p := store.TransactionPatch{Version: in.Version, RequestID: in.RequestID, Amount: in.Amount, Category: in.Category, GoalID: in.GoalID}
	if in.Amount != nil {
		if err := ledger.ValidateAmount(*in.Amount); err != nil {
			validation(w, "amount", err.Error())
			return
		}
	}
	if in.Note != nil {
		note, err := ledger.ValidateNote(*in.Note)
		if err != nil {
			validation(w, "note", err.Error())
			return
		}
		p.Note = &note
	}
	if in.Date != nil {
		d, msg := parseDate(*in.Date)
		if msg != "" {
			validation(w, "date", msg)
			return
		}
		p.Date = &d
	}

	uid := userFrom(r.Context()).ID
	id := r.PathValue("id")
	var cur store.Transaction
	if in.Category != nil || in.GoalID != nil {
		// Kind decides whether a category or goal is allowed; read the current record.
		var err error
		if cur, err = s.store.GetTransaction(r.Context(), uid, id); err != nil {
			writeStoreError(w, r, err)
			return
		}
	}
	if in.GoalID != nil && *in.GoalID != cur.GoalID {
		// Keeping the current goal is fine even if it was since marked done; a new link must be active.
		if msg, err := s.checkGoal(r, cur.Kind, *in.GoalID); err != nil {
			writeStoreError(w, r, err)
			return
		} else if msg != "" {
			validation(w, "goalId", msg)
			return
		}
	}
	if in.Category != nil {
		if !cur.Kind.NeedsCategory() {
			validation(w, "category", "Для этой записи категория не нужна")
			return
		}
		ok, err := s.usableCategory(r, *in.Category)
		if err != nil {
			writeStoreError(w, r, err)
			return
		}
		if !ok {
			validation(w, "category", "Выберите категорию из списка")
			return
		}
	}

	t, err := s.store.UpdateTransaction(r.Context(), uid, id, p)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	sum, err := s.buildSummary(r.Context(), uid, ledger.MonthKey(ledger.Now()))
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transaction": toDTO(t), "summary": sum})
}

// handleDeleteTransaction deletes a record; idempotent (204 when already gone).
func (s *Server) handleDeleteTransaction(w http.ResponseWriter, r *http.Request) {
	ver, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || ver < 1 {
		validation(w, "version", "Не указана версия записи")
		return
	}
	uid := userFrom(r.Context()).ID
	deleted, err := s.store.DeleteTransaction(r.Context(), uid, r.PathValue("id"), ver)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	if !deleted {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	sum, err := s.buildSummary(r.Context(), uid, ledger.MonthKey(ledger.Now()))
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": sum})
}

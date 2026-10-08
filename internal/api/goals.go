package api

import (
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

type goalDTO struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Target   int64   `json:"target"`
	Quarter  string  `json:"quarter"`
	Status   string  `json:"status"`
	Note     string  `json:"note"`
	Progress float64 `json:"progress"`
}

func goalToDTO(g store.Goal, savings int64) goalDTO {
	status := g.Status
	if status != "done" {
		status = "active"
	}
	p := 0.0
	if g.Target > 0 && savings > 0 {
		p = min(float64(savings)/float64(g.Target), 1)
	}
	return goalDTO{ID: g.ID, Name: g.Name, Target: g.Target, Quarter: g.Quarter, Status: status, Note: g.Note, Progress: p}
}

func (s *Server) handleListGoals(w http.ResponseWriter, r *http.Request) {
	uid := userFrom(r.Context()).ID
	goals, err := s.store.ListGoals(r.Context(), uid)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	u, err := s.store.GetUser(r.Context(), uid)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	items := make([]goalDTO, 0, len(goals))
	for _, g := range goals {
		items = append(items, goalToDTO(g, u.SavingsBalance))
	}
	writeJSON(w, http.StatusOK, map[string]any{"savingsBalance": u.SavingsBalance, "items": items})
}

type newGoal struct {
	ClientID string `json:"clientId"`
	Name     string `json:"name"`
	Target   int64  `json:"target"`
	Quarter  string `json:"quarter"`
	Note     string `json:"note"`
}

func (s *Server) handleAddGoal(w http.ResponseWriter, r *http.Request) {
	var in newGoal
	if err := decodeJSON(r, &in); err != nil {
		validation(w, "", err.Error())
		return
	}
	if !uuidRe.MatchString(in.ClientID) {
		validation(w, "clientId", "Некорректный идентификатор формы")
		return
	}
	name := strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 60 {
		validation(w, "name", "Название: от 1 до 60 символов")
		return
	}
	if err := ledger.ValidateAmount(in.Target); err != nil {
		validation(w, "target", err.Error())
		return
	}
	if !slices.Contains(ledger.AllowedQuarters(ledger.Now()), in.Quarter) {
		validation(w, "quarter", "Выберите текущий или один из следующих трёх кварталов")
		return
	}
	note, err := ledger.ValidateNote(in.Note)
	if err != nil {
		validation(w, "note", err.Error())
		return
	}

	uid := userFrom(r.Context()).ID
	g, created, err := s.store.AddGoal(r.Context(), uid, store.Goal{
		ID: in.ClientID, Name: name, Target: in.Target, Quarter: in.Quarter,
		Status: "active", Note: note, CreatedAt: time.Now(),
	})
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	u, err := s.store.GetUser(r.Context(), uid)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, goalToDTO(g, u.SavingsBalance))
}

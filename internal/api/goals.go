package api

import (
	"net/http"
	"slices"
	"strconv"
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
	Saved    int64   `json:"saved"`
	Progress float64 `json:"progress"`
	Version  int64   `json:"version"`
}

// goalToDTO: saved is the sum of deposits linked to the goal; progress = saved / target, capped at 1.
func goalToDTO(g store.Goal, saved int64) goalDTO {
	status := g.Status
	if status != "done" {
		status = "active"
	}
	p := 0.0
	if g.Target > 0 && saved > 0 {
		p = min(float64(saved)/float64(g.Target), 1)
	}
	v := g.Version
	if v == 0 {
		v = 1
	}
	return goalDTO{ID: g.ID, Name: g.Name, Target: g.Target, Quarter: g.Quarter, Status: status, Note: g.Note, Saved: saved, Progress: p, Version: v}
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
	saved, err := s.store.GoalSavings(r.Context(), uid)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	items := make([]goalDTO, 0, len(goals))
	for _, g := range goals {
		items = append(items, goalToDTO(g, saved[g.ID]))
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
		Status: "active", Note: note, CreatedAt: time.Now(), Version: 1,
	})
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	saved, err := s.store.GoalSavings(r.Context(), uid)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, goalToDTO(g, saved[g.ID]))
}

type goalPatch struct {
	Version   int64   `json:"version"`
	RequestID string  `json:"requestId"`
	Name      *string `json:"name"`
	Target    *int64  `json:"target"`
	Quarter   *string `json:"quarter"`
	Note      *string `json:"note"`
	Status    *string `json:"status"`
}

// handlePatchGoal edits a goal or toggles its status (FR-013).
func (s *Server) handlePatchGoal(w http.ResponseWriter, r *http.Request) {
	var in goalPatch
	if err := decodeJSON(r, &in); err != nil {
		validation(w, "", err.Error())
		return
	}
	if in.Version < 1 {
		validation(w, "version", "Не указана версия цели")
		return
	}
	if !uuidRe.MatchString(in.RequestID) {
		validation(w, "requestId", "Некорректный идентификатор запроса")
		return
	}
	p := store.GoalPatch{Version: in.Version, RequestID: in.RequestID, Target: in.Target, Status: in.Status}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if n := utf8.RuneCountInString(name); n < 1 || n > 60 {
			validation(w, "name", "Название: от 1 до 60 символов")
			return
		}
		p.Name = &name
	}
	if in.Target != nil {
		if err := ledger.ValidateAmount(*in.Target); err != nil {
			validation(w, "target", err.Error())
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
	if in.Status != nil && *in.Status != "active" && *in.Status != "done" {
		validation(w, "status", "Статус: active или done")
		return
	}

	uid := userFrom(r.Context()).ID
	id := r.PathValue("id")
	if in.Quarter != nil {
		// The goal's own (possibly past) quarter stays valid; a new one must be current + next 3.
		goals, err := s.store.ListGoals(r.Context(), uid)
		if err != nil {
			writeStoreError(w, r, err)
			return
		}
		ownQuarter := ""
		for _, g := range goals {
			if g.ID == id {
				ownQuarter = g.Quarter
			}
		}
		if *in.Quarter != ownQuarter && !slices.Contains(ledger.AllowedQuarters(ledger.Now()), *in.Quarter) {
			validation(w, "quarter", "Выберите текущий или один из следующих трёх кварталов")
			return
		}
		p.Quarter = in.Quarter
	}

	g, err := s.store.UpdateGoal(r.Context(), uid, id, p)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	saved, err := s.store.GoalSavings(r.Context(), uid)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, goalToDTO(g, saved[g.ID]))
}

// handleDeleteGoal deletes a goal; idempotent.
func (s *Server) handleDeleteGoal(w http.ResponseWriter, r *http.Request) {
	ver, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || ver < 1 {
		validation(w, "version", "Не указана версия цели")
		return
	}
	if err := s.store.DeleteGoal(r.Context(), userFrom(r.Context()).ID, r.PathValue("id"), ver); err != nil {
		writeStoreError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

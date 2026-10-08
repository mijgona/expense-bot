package api

import (
	"net/http"
	"time"

	"expense-bot/internal/ledger"
)

type sessionUser struct {
	ID           int64     `json:"id"`
	FirstName    string    `json:"firstName"`
	DisplayName  string    `json:"displayName"`
	Username     string    `json:"username"`
	RegisteredAt time.Time `json:"registeredAt"`
	IsNew        bool      `json:"isNew"`
}

type sessionResponse struct {
	User              sessionUser   `json:"user"`
	Salary            int64         `json:"salary"`
	Categories        []categoryDTO `json:"categories"`
	CategoriesVersion int64         `json:"categoriesVersion"`
	CurrentMonth      string        `json:"currentMonth"`
	FirstMonth        *string       `json:"firstMonth"`
}

// handleSession registers the user on launch (FR-002) and returns bootstrap data.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	tu := userFrom(r.Context())
	u, created, err := s.store.EnsureUser(r.Context(), tu.ID, tu.FirstName, tu.Username)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	first, err := s.store.FirstMonth(r.Context(), tu.ID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}

	eff := s.effective(u)
	resp := sessionResponse{
		User: sessionUser{
			ID: u.ID, FirstName: u.FirstName, DisplayName: eff.DisplayName, Username: u.Username,
			RegisteredAt: u.RegisteredAt, IsNew: created,
		},
		Salary:       eff.Salary,
		CurrentMonth: ledger.MonthKey(ledger.Now()),
	}
	if first != "" {
		resp.FirstMonth = &first
	}
	l := listDTO(u.CategoryList(), u.CategoriesVersion)
	resp.Categories, resp.CategoriesVersion = l.Items, l.Version
	writeJSON(w, http.StatusOK, resp)
}

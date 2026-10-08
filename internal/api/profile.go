package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"expense-bot/internal/ledger"
	"expense-bot/internal/payroll"
	"expense-bot/internal/store"
)

type profileString struct {
	Value     string `json:"value"`
	Default   string `json:"default"`
	IsDefault bool   `json:"isDefault"`
}

type profileInt struct {
	Value     int64 `json:"value"`
	Default   int64 `json:"default"`
	IsDefault bool  `json:"isDefault"`
}

type profileDTO struct {
	Telegram struct {
		FirstName string `json:"firstName"`
		Username  string `json:"username"`
	} `json:"telegram"`
	DisplayName     profileString `json:"displayName"`
	Salary          profileInt    `json:"salary"`
	SalaryMode      string        `json:"salaryMode"`
	Advance         profileInt    `json:"advance"`
	Rest            int64         `json:"rest"`
	SalaryReminders bool          `json:"salaryReminders"`
	UpdatedAt       *time.Time    `json:"updatedAt"`
}

// effective merges the user's overrides with the shared defaults (research R7).
func (s *Server) effective(u store.User) ledger.Effective {
	return ledger.NewEffective(u.FirstName, s.cfg.Salary, u.Overrides())
}

func (s *Server) profileOf(u store.User) profileDTO {
	e := s.effective(u)
	var p profileDTO
	p.Telegram.FirstName, p.Telegram.Username = u.FirstName, u.Username
	p.DisplayName = profileString{e.DisplayName, e.DisplayNameDefault, e.DisplayNameIsDefault}
	p.Salary = profileInt{e.Salary, e.SalaryDefault, e.SalaryIsDefault}
	mode, _ := payroll.ParseMode(u.SalaryMode)
	p.SalaryMode = string(mode)
	half := payroll.EffectiveAdvance(e.Salary, nil)
	adv := payroll.EffectiveAdvance(e.Salary, u.Advance)
	p.Advance = profileInt{adv, half, u.Advance == nil || adv != *u.Advance}
	p.Rest = e.Salary
	if mode == payroll.ModeSplit {
		p.Rest = e.Salary - adv
	}
	p.SalaryReminders = !u.SalaryRemindersOff
	p.UpdatedAt = u.ProfileUpdatedAt
	return p
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	u, err := s.store.GetUser(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.profileOf(u))
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// handlePatchProfile changes overrides; null resets a value to the shared default.
func (s *Server) handlePatchProfile(w http.ResponseWriter, r *http.Request) {
	var in map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		validation(w, "", "Некорректный запрос")
		return
	}
	var p store.ProfilePatch
	for key, raw := range in {
		switch key {
		case "displayName":
			p.DisplayName.Set = true
			if isNull(raw) {
				continue
			}
			var v string
			if json.Unmarshal(raw, &v) != nil {
				validation(w, "displayName", "Имя должно быть строкой")
				return
			}
			v = strings.TrimSpace(v)
			if n := utf8.RuneCountInString(v); n < 1 || n > 40 {
				validation(w, "displayName", "Имя: от 1 до 40 символов")
				return
			}
			p.DisplayName.Value = &v
		case "salary":
			p.Salary.Set = true
			if isNull(raw) {
				continue
			}
			var v int64
			if json.Unmarshal(raw, &v) != nil || ledger.ValidateAmount(v) != nil {
				validation(w, "salary", "Зарплата: от 0,01 до 10 000 000 с.")
				return
			}
			p.Salary.Value = &v
		case "salaryMode":
			var v string
			if json.Unmarshal(raw, &v) != nil {
				validation(w, "salaryMode", "Режим: single или split")
				return
			}
			mode, err := payroll.ParseMode(v)
			if err != nil || v == "" {
				validation(w, "salaryMode", "Режим: single или split")
				return
			}
			m := string(mode)
			p.SalaryMode = &m
		case "advance":
			p.Advance.Set = true
			if isNull(raw) {
				continue
			}
			var v int64
			if json.Unmarshal(raw, &v) != nil {
				validation(w, "advance", "Аванс должен быть числом")
				return
			}
			p.Advance.Value = &v
		case "salaryReminders":
			var v bool
			if json.Unmarshal(raw, &v) != nil {
				validation(w, "salaryReminders", "Ожидается true или false")
				return
			}
			p.SalaryReminders = &v
		default:
			validation(w, key, "Неизвестное поле")
			return
		}
	}
	if p.Advance.Value != nil {
		// Validate against the salary after this patch (FR-002: 1 с. … salary − 1 с.).
		cur, err := s.store.GetUser(r.Context(), userFrom(r.Context()).ID)
		if err != nil {
			writeStoreError(w, r, err)
			return
		}
		salary := s.effective(cur).Salary
		if p.Salary.Set {
			if p.Salary.Value != nil {
				salary = *p.Salary.Value
			} else {
				salary = s.effective(store.User{FirstName: cur.FirstName}).Salary
			}
		}
		if a := *p.Advance.Value; a < ledger.PerSomoni || a > salary-ledger.PerSomoni {
			validation(w, "advance", "Аванс: от 1 с. до зарплаты минус 1 с.")
			return
		}
	}
	u, err := s.store.UpdateProfile(r.Context(), userFrom(r.Context()).ID, p)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.profileOf(u))
}

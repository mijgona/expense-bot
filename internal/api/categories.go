package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"expense-bot/internal/catalog"
	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

type categoryDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Limit     int64  `json:"limit"`
	Hidden    bool   `json:"hidden"`
	Position  int    `json:"position"`
	IsDefault bool   `json:"isDefault"`
}

type categoryListDTO struct {
	Version int64         `json:"version"`
	Items   []categoryDTO `json:"items"`
}

func categoryDTOs(l catalog.List) []categoryDTO {
	out := []categoryDTO{}
	for _, v := range l.Sorted() {
		out = append(out, categoryDTO{ID: v.ID, Name: v.Name, Limit: v.Limit, Hidden: v.Hidden, Position: v.Position, IsDefault: v.DefaultKey != ""})
	}
	return out
}

func listDTO(l catalog.List, version int64) categoryListDTO {
	if version == 0 {
		version = 1
	}
	return categoryListDTO{Version: version, Items: categoryDTOs(l)}
}

// categoryInfos converts the user's list for ledger.BuildCategoryLines.
func categoryInfos(l catalog.List) []ledger.CategoryInfo {
	out := make([]ledger.CategoryInfo, 0, len(l))
	for _, v := range l.Sorted() {
		out = append(out, ledger.CategoryInfo{ID: v.ID, Label: v.Name, Limit: v.Limit, Hidden: v.Hidden, Position: v.Position})
	}
	return out
}

// writeCategoryError maps catalog / store errors of list operations.
func writeCategoryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrDuplicate):
		writeError(w, http.StatusConflict, "duplicate", "Такая категория уже есть", "name")
	case errors.Is(err, catalog.ErrInvalidName):
		writeError(w, http.StatusBadRequest, "validation", "Название: от 1 до 30 символов", "name")
	case errors.Is(err, catalog.ErrInvalidLimit):
		writeError(w, http.StatusBadRequest, "validation", "Лимит: от 0 до 10 000 000 с. (0 — без лимита)", "limit")
	case errors.Is(err, catalog.ErrLimitReached):
		writeError(w, http.StatusUnprocessableEntity, "limit_reached", "Можно не больше 50 категорий", "")
	case errors.Is(err, catalog.ErrLastVisible):
		writeError(w, http.StatusUnprocessableEntity, "last_visible", "Должна остаться хотя бы одна видимая категория", "hidden")
	case errors.Is(err, catalog.ErrBadOrder):
		writeError(w, http.StatusBadRequest, "validation", "Порядок должен содержать все категории ровно один раз", "ids")
	case errors.Is(err, catalog.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Категория не найдена", "")
	default:
		var ce *store.ConflictError
		if errors.As(err, &ce) {
			if st, ok := ce.Current.(store.CategoryState); ok {
				writeJSON(w, http.StatusConflict, errorBody{Error: errorDetail{
					Code: "conflict", Message: "Список изменили на другом устройстве", Current: listDTO(st.List, st.Version)}})
				return
			}
		}
		writeStoreError(w, r, err)
	}
}

func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	u, err := s.store.GetUser(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listDTO(u.CategoryList(), u.CategoriesVersion))
}

type newCategory struct {
	ClientID string `json:"clientId"`
	Name     string `json:"name"`
	Limit    int64  `json:"limit"`
}

// handleAddCategory adds a category; idempotent by clientId (the ID derives from it).
func (s *Server) handleAddCategory(w http.ResponseWriter, r *http.Request) {
	var in newCategory
	if err := decodeJSON(r, &in); err != nil {
		validation(w, "", err.Error())
		return
	}
	if !uuidRe.MatchString(in.ClientID) {
		validation(w, "clientId", "Некорректный идентификатор формы")
		return
	}
	id := catalog.ClientID(in.ClientID)
	created := false
	list, ver, err := s.store.UpdateCategories(r.Context(), userFrom(r.Context()).ID, 0, func(l catalog.List) error {
		if _, ok := l.Get(id); ok {
			return nil // replay of an already created category
		}
		created = true
		return l.Add(id, in.Name, in.Limit, time.Now().UTC())
	})
	if err != nil {
		writeCategoryError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, listDTO(list, ver))
}

type categoryPatch struct {
	Version int64   `json:"version"`
	Name    *string `json:"name"`
	Limit   *int64  `json:"limit"`
	Hidden  *bool   `json:"hidden"`
}

// handlePatchCategory renames, changes the limit or hides/shows a category. No delete.
func (s *Server) handlePatchCategory(w http.ResponseWriter, r *http.Request) {
	var in categoryPatch
	if err := decodeJSON(r, &in); err != nil {
		validation(w, "", err.Error())
		return
	}
	if in.Version < 1 {
		validation(w, "version", "Не указана версия списка")
		return
	}
	id := r.PathValue("id")
	list, ver, err := s.store.UpdateCategories(r.Context(), userFrom(r.Context()).ID, in.Version, func(l catalog.List) error {
		if _, ok := l.Get(id); !ok {
			return catalog.ErrNotFound
		}
		if in.Name != nil {
			if err := l.Rename(id, *in.Name); err != nil {
				return err
			}
		}
		if in.Limit != nil {
			if err := l.SetLimit(id, *in.Limit); err != nil {
				return err
			}
		}
		if in.Hidden != nil {
			return l.SetHidden(id, *in.Hidden)
		}
		return nil
	})
	if err != nil {
		writeCategoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listDTO(list, ver))
}

type categoryOrder struct {
	Version int64    `json:"version"`
	IDs     []string `json:"ids"`
}

// handleOrderCategories sets the order of all categories (FR-014).
func (s *Server) handleOrderCategories(w http.ResponseWriter, r *http.Request) {
	var in categoryOrder
	if err := decodeJSON(r, &in); err != nil {
		validation(w, "", err.Error())
		return
	}
	if in.Version < 1 {
		validation(w, "version", "Не указана версия списка")
		return
	}
	list, ver, err := s.store.UpdateCategories(r.Context(), userFrom(r.Context()).ID, in.Version, func(l catalog.List) error {
		return l.Reorder(in.IDs)
	})
	if err != nil {
		writeCategoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listDTO(list, ver))
}

// usableCategory reports whether id is an existing, visible category of the user (FR-010).
func (s *Server) usableCategory(r *http.Request, id string) (bool, error) {
	u, err := s.store.GetUser(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		return false, err
	}
	return u.CategoryList().Usable(strings.TrimSpace(id)), nil
}

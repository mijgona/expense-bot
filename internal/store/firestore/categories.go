package firestore

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"

	"expense-bot/internal/catalog"
	"expense-bot/internal/store"
)

// UpdateCategories is a transactional read-modify-write of users/{id}.categories (research R1).
func (s *Store) UpdateCategories(ctx context.Context, userID int64, version int64, fn func(catalog.List) error) (catalog.List, int64, error) {
	ref := s.user(userID)
	var (
		out    catalog.List
		outVer int64
	)
	err := s.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		var u store.User
		snap, err := tx.Get(ref)
		if err != nil && !notFound(err) {
			return err
		}
		if snap != nil && snap.Exists() {
			if err := snap.DataTo(&u); err != nil {
				return err
			}
		}
		list := u.Categories.Clone()
		if len(list) == 0 {
			list = catalog.Defaults(time.Now().UTC())
		}
		cur := u.CategoriesVersion
		if cur == 0 {
			cur = 1
		}
		if version != 0 && version != cur {
			return &store.ConflictError{Current: store.CategoryState{List: u.CategoryList(), Version: cur}}
		}
		if err := fn(list); err != nil {
			return err
		}
		out, outVer = list, cur+1
		// Overwrite the whole field (keys are never removed, but values must not merge).
		return tx.Set(ref, map[string]any{"categories": list, "categoriesVersion": outVer}, firestore.Merge([]string{"categories"}, []string{"categoriesVersion"}))
	})
	if err != nil {
		return nil, 0, wrapTxErr("update categories", err)
	}
	return out, outVer, nil
}

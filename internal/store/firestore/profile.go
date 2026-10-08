package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"

	"expense-bot/internal/store"
)

// UpdateProfile applies profile overrides; a nil value resets that setting to the default.
// Category limits live in the category list since feature 006.
func (s *Store) UpdateProfile(ctx context.Context, userID int64, p store.ProfilePatch) (store.User, error) {
	ref := s.user(userID)
	// Make sure the document exists so Update does not fail with NotFound.
	if _, err := ref.Set(ctx, map[string]any{"profileUpdatedAt": time.Now().UTC()}, firestore.MergeAll); err != nil {
		return store.User{}, fmt.Errorf("update profile: %w", err)
	}

	var ups []firestore.Update
	if p.DisplayName.Set {
		var v any = firestore.Delete
		if p.DisplayName.Value != nil {
			v = *p.DisplayName.Value
		}
		ups = append(ups, firestore.Update{Path: "displayName", Value: v})
	}
	if p.Salary.Set {
		var v any = firestore.Delete
		if p.Salary.Value != nil {
			v = *p.Salary.Value
		}
		ups = append(ups, firestore.Update{Path: "salary", Value: v})
	}
	if len(ups) > 0 {
		if _, err := ref.Update(ctx, ups); err != nil {
			return store.User{}, fmt.Errorf("update profile: %w", err)
		}
	}
	return s.GetUser(ctx, userID)
}

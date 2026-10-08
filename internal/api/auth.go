package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"expense-bot/internal/tgauth"
)

const initDataMaxAge = 24 * time.Hour

type ctxKey struct{}

// userFrom returns the verified user put in the context by auth.
func userFrom(ctx context.Context) tgauth.User {
	u, _ := ctx.Value(ctxKey{}).(tgauth.User)
	return u
}

// auth verifies `Authorization: tma <initData>` and the allowed-users list.
// Handlers must take the user ID only from userFrom(ctx).
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var u tgauth.User
		if s.cfg.DevUserID != 0 {
			u = tgauth.User{ID: s.cfg.DevUserID, FirstName: "Dev"}
		} else {
			initData, ok := strings.CutPrefix(r.Header.Get("Authorization"), "tma ")
			if !ok || initData == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Откройте приложение заново из Telegram", "")
				return
			}
			var err error
			u, err = tgauth.Verify(initData, s.cfg.BotToken, initDataMaxAge, time.Now())
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Откройте приложение заново из Telegram", "")
				return
			}
		}
		if len(s.cfg.AllowedUserIDs) > 0 && !slices.Contains(s.cfg.AllowedUserIDs, u.ID) {
			writeError(w, http.StatusForbidden, "forbidden", "⛔ Доступ запрещён", "")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

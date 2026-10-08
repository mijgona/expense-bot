package api

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

const advisorTimeout = 90 * time.Second

// handleAdvisorReport generates the AI report on demand (FR-016), one at a time per user.
func (s *Server) handleAdvisorReport(w http.ResponseWriter, r *http.Request) {
	if s.advisor == nil || !s.advisor.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "advisor_unavailable", "ИИ-советник не настроен: нужен GEMINI_API_KEY", "")
		return
	}
	uid := userFrom(r.Context()).ID
	mu, _ := s.advisorLocks.LoadOrStore(uid, &sync.Mutex{})
	lock := mu.(*sync.Mutex)
	if !lock.TryLock() {
		writeError(w, http.StatusConflict, "busy", "Отчёт уже готовится, подождите", "")
		return
	}
	defer lock.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), advisorTimeout)
	defer cancel()
	md, err := s.advisor.Generate(ctx, uid)
	if err != nil {
		log.Printf("api: advisor user %d: %v", uid, err)
		writeError(w, http.StatusServiceUnavailable, "advisor_unavailable", "Не удалось сформировать отчёт, попробуйте позже", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"markdown": md, "generatedAt": time.Now().UTC()})
}

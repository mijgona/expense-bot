package bot

import "sync"

// dialogStep represents the current step in a multi-turn conversation.
type dialogStep int

const (
	stepNone        dialogStep = iota
	stepChooseCat              // waiting for category selection (button press)
	stepEnterAmount            // waiting for "amount [description]" text
	stepEnterIncome            // waiting for income amount [description]
)

// dialog holds per-user conversation state.
type dialog struct {
	Step        dialogStep
	Category    string  // chosen category name
	Amount      float64 // pre-filled amount (quick input)
	Description string  // pre-filled description (quick input)
}

// stateStore is a thread-safe in-memory map of chat ID → dialog.
type stateStore struct {
	mu    sync.Mutex
	store map[int64]*dialog
}

func newStateStore() *stateStore {
	return &stateStore{store: make(map[int64]*dialog)}
}

func (s *stateStore) get(chatID int64) *dialog {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store[chatID]
}

func (s *stateStore) set(chatID int64, d *dialog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[chatID] = d
}

func (s *stateStore) clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, chatID)
}

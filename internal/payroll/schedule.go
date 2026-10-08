package payroll

import (
	"expense-bot/internal/ledger"
	"expense-bot/internal/store"
)

// Schedule returns a user's effective salary schedule: mode, salary and advance (diram).
// In single mode advance is 0.
func Schedule(u store.User, cfgSalarySomoni int) (mode Mode, salary, advance int64) {
	salary = ledger.NewEffective(u.FirstName, cfgSalarySomoni, u.Overrides()).Salary
	mode, _ = ParseMode(u.SalaryMode)
	if mode == ModeSplit {
		advance = EffectiveAdvance(salary, u.Advance)
	}
	return mode, salary, advance
}

// ExpectedFor returns the user's expected payments for a month.
func ExpectedFor(u store.User, cfgSalarySomoni int, month string) []Payment {
	mode, salary, advance := Schedule(u, cfgSalarySomoni)
	return Expected(mode, salary, advance, month)
}

package ledger

import "strings"

// ProfileOverrides are a user's personal settings; nil means "use the default".
// Category limits moved to the user's category list in feature 006.
type ProfileOverrides struct {
	DisplayName *string
	Salary      *int64 // diram
}

// Effective is the profile with defaults filled in.
type Effective struct {
	DisplayName          string
	DisplayNameDefault   string
	DisplayNameIsDefault bool

	Salary          int64
	SalaryDefault   int64
	SalaryIsDefault bool
}

// NewEffective merges overrides over the shared defaults (Telegram first name, cfg salary in somoni).
func NewEffective(firstName string, cfgSalarySomoni int, o ProfileOverrides) Effective {
	e := Effective{
		DisplayName: firstName, DisplayNameDefault: firstName, DisplayNameIsDefault: true,
		SalaryDefault: int64(cfgSalarySomoni) * PerSomoni,
	}
	if o.DisplayName != nil && strings.TrimSpace(*o.DisplayName) != "" {
		e.DisplayName, e.DisplayNameIsDefault = strings.TrimSpace(*o.DisplayName), false
	}
	e.Salary, e.SalaryIsDefault = e.SalaryDefault, true
	if o.Salary != nil {
		e.Salary, e.SalaryIsDefault = *o.Salary, false
	}
	return e
}

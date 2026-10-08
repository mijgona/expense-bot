package ledger

import (
	"strings"

	"expense-bot/internal/category"
)

// ProfileOverrides are a user's personal settings; nil / missing keys mean "use the default".
type ProfileOverrides struct {
	DisplayName *string
	Salary      *int64           // diram
	Limits      map[string]int64 // diram per category name; 0 = no limit
}

// Effective is the profile with defaults filled in (research R7).
type Effective struct {
	DisplayName          string
	DisplayNameDefault   string
	DisplayNameIsDefault bool

	Salary          int64
	SalaryDefault   int64
	SalaryIsDefault bool

	Limits         map[string]int64 // every configured category
	LimitDefaults  map[string]int64
	LimitIsDefault map[string]bool
}

// NewEffective merges overrides over the shared defaults (Telegram first name, cfg salary
// in somoni, category limits). Unknown categories in overrides are ignored.
func NewEffective(firstName string, cfgSalarySomoni int, o ProfileOverrides) Effective {
	e := Effective{
		DisplayName:          firstName,
		DisplayNameDefault:   firstName,
		DisplayNameIsDefault: true,
		SalaryDefault:        int64(cfgSalarySomoni) * PerSomoni,
		Limits:               map[string]int64{},
		LimitDefaults:        map[string]int64{},
		LimitIsDefault:       map[string]bool{},
	}
	if o.DisplayName != nil && strings.TrimSpace(*o.DisplayName) != "" {
		e.DisplayName, e.DisplayNameIsDefault = strings.TrimSpace(*o.DisplayName), false
	}
	e.Salary, e.SalaryIsDefault = e.SalaryDefault, true
	if o.Salary != nil {
		e.Salary, e.SalaryIsDefault = *o.Salary, false
	}
	for _, c := range category.All() {
		def := int64(c.Limit) * PerSomoni
		e.LimitDefaults[c.Name] = def
		if v, ok := o.Limits[c.Name]; ok {
			e.Limits[c.Name], e.LimitIsDefault[c.Name] = v, false
		} else {
			e.Limits[c.Name], e.LimitIsDefault[c.Name] = def, true
		}
	}
	return e
}

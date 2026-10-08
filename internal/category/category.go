package category

// Category is one of the shared default categories every user starts with (feature 006: users
// can rename, hide and add their own; this list only seeds them and resolves legacy names).
type Category struct {
	Key   string // stable key; the user's category ID is "c_"+Key
	Name  string // legacy internal name stored in old records (resolved by the 006 conversion)
	Label string // default display name (with emoji)
	Limit int    // default monthly limit in somoni
}

// All returns the default categories in their default order.
func All() []Category {
	return []Category{
		{Key: "food", Name: "Еда", Label: "🍽 Еда/Продукты", Limit: 2000},
		{Key: "rent", Name: "Аренда", Label: "🏠 Аренда/ЖКХ", Limit: 300},
		{Key: "transport", Name: "Транспорт", Label: "🚗 Транспорт", Limit: 1000},
		{Key: "clothes", Name: "Одежда", Label: "👗 Одежда", Limit: 1000},
		{Key: "health", Name: "Здоровье", Label: "💊 Здоровье", Limit: 500},
		{Key: "phone", Name: "Связь", Label: "📱 Связь", Limit: 200},
		{Key: "fun", Name: "Развлечения", Label: "🎭 Развлечения", Limit: 500},
		{Key: "savings", Name: "Накопления", Label: "💰 Накопления", Limit: 5000},
		{Key: "study", Name: "Обучение", Label: "📚 Обучение", Limit: 200},
		{Key: "kids_study", Name: "Обучение детей", Label: "🎒 Обучение детей", Limit: 3000},
		{Key: "self", Name: "На себя", Label: "💄 На себя", Limit: 1000},
		{Key: "business", Name: "Бизнес", Label: "💼 Бизнес", Limit: 300},
		{Key: "other", Name: "Другое", Label: "🔧 Другое", Limit: 500},
		{Key: "unknown", Name: "Не знаю", Label: "🤷 Не знаю на что потратила", Limit: 450},
	}
}

// DefaultID returns the category ID of a default key.
func DefaultID(key string) string { return "c_" + key }

// ByName returns a default category by its legacy internal name, or nil.
func ByName(name string) *Category {
	for _, c := range All() {
		if c.Name == name {
			return &c
		}
	}
	return nil
}

// FindByName is kept for the legacy Sheets mapping; same as ByName.
func FindByName(name string) *Category { return ByName(name) }

// FindByLabel returns a default category by its display label, or nil.
func FindByLabel(label string) *Category {
	for _, c := range All() {
		if c.Label == label {
			return &c
		}
	}
	return nil
}

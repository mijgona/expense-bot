package category

// Category represents an expense category with its display label and monthly budget limit.
type Category struct {
	Name  string // internal name stored in Google Sheets
	Label string // emoji label shown in Telegram
	Limit int    // monthly budget limit in somoni
}

// All returns the full list of expense categories.
func All() []Category {
	return []Category{
		{Name: "Еда", Label: "🍽 Еда/Продукты", Limit: 3500},
		{Name: "Аренда", Label: "🏠 Аренда/ЖКХ", Limit: 4000},
		{Name: "Транспорт", Label: "🚗 Транспорт", Limit: 1000},
		{Name: "Одежда", Label: "👗 Одежда", Limit: 800},
		{Name: "Здоровье", Label: "💊 Здоровье", Limit: 800},
		{Name: "Связь", Label: "📱 Связь", Limit: 500},
		{Name: "Развлечения", Label: "🎭 Развлечения", Limit: 700},
		{Name: "Накопления", Label: "💰 Накопления", Limit: 2000},
		{Name: "Обучение", Label: "📚 Обучение", Limit: 1500},
		{Name: "Обучение детей", Label: "🎒 Обучение детей", Limit: 1500},
		{Name: "На себя", Label: "💄 На себя", Limit: 700},
		{Name: "Другое", Label: "🔧 Другое", Limit: 700},
	}
}

// FindByName returns a category by its internal name, or nil if not found.
func FindByName(name string) *Category {
	for _, c := range All() {
		if c.Name == name {
			return &c
		}
	}
	return nil
}

// FindByLabel returns a category by its display label, or nil if not found.
func FindByLabel(label string) *Category {
	for _, c := range All() {
		if c.Label == label {
			return &c
		}
	}
	return nil
}

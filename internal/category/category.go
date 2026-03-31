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
		{Name: "Еда", Label: "🍽 Еда/Продукты", Limit: 2000},
		{Name: "Аренда", Label: "🏠 Аренда/ЖКХ", Limit: 300},
		{Name: "Транспорт", Label: "🚗 Транспорт", Limit: 1000},
		{Name: "Одежда", Label: "👗 Одежда", Limit: 1000},
		{Name: "Здоровье", Label: "💊 Здоровье", Limit: 500},
		{Name: "Связь", Label: "📱 Связь", Limit: 200},
		{Name: "Развлечения", Label: "🎭 Развлечения", Limit: 500},
		{Name: "Накопления", Label: "💰 Накопления", Limit: 5000},
		{Name: "Обучение", Label: "📚 Обучение", Limit: 200},
		{Name: "Обучение детей", Label: "🎒 Обучение детей", Limit: 3000},
		{Name: "На себя", Label: "💄 На себя", Limit: 1000},
		{Name: "Бизнес", Label: "💼 Бизнес", Limit: 300},
		{Name: "Другое", Label: "🔧 Другое", Limit: 500},
		{Name: "Не знаю", Label: "🤷 Не знаю на что потратила", Limit: 450},
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

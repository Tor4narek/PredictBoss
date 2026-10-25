package domain

// MetricCatalogOrder is the display and calculation order of the supported
// company metric codes. Unknown codes remain valid data and sort after this
// catalog, ordered by their code and then metric ID.
func MetricCatalogOrder(code string) (int, bool) {
	order, ok := metricCatalogOrder[code]
	return order, ok
}

var metricCatalogOrder = map[string]int{
	"ставка":      1,
	"депозит":     2,
	"курс":        3,
	"логистика":   4,
	"цена":        5,
	"спрос":       6,
	"запас":       7,
	"контрагент":  8,
	"зарплата":    9,
}

// Package fixtures содержит демонстрационные данные симулятора магазина ОМА
// и профилей исполнителей. Все синтетические данные помечены признаком demo
// (разделы 2.1 и 11 продуктового ТЗ).
package fixtures

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"oma/internal/integrations/oma"
	"oma/internal/prorab"
)

//go:embed products.json
var productsRaw []byte

//go:embed foremen.json
var foremenRaw []byte

// Products возвращает каталог демо-магазина.
func Products() []oma.Product {
	var out []oma.Product
	if err := json.Unmarshal(productsRaw, &out); err != nil {
		panic(fmt.Sprintf("fixtures: некорректный products.json: %v", err))
	}
	return out
}

// Foremen возвращает демо-профили исполнителей.
func Foremen() []prorab.Foreman {
	var out []prorab.Foreman
	if err := json.Unmarshal(foremenRaw, &out); err != nil {
		panic(fmt.Sprintf("fixtures: некорректный foremen.json: %v", err))
	}
	return out
}

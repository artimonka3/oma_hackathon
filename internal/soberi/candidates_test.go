package soberi

import (
	"oma/internal/integrations/oma"
	"testing"
)

func TestRecommendationCategoryFollowsTask(t *testing.T) {
	products := []oma.Product{{SKU: "paint", Material: "paint", Title: "Краска для стен", Unit: "l", PackAmount: 10, Coverage: .1, PriceMinor: 1000, Stock: 50}, {SKU: "tile", Material: "tiling", Unit: "m2", PackAmount: 1, PriceMinor: 1000, Stock: 50}, {SKU: "laminate", Material: "flooring", OmaCategory: "Ламинат", Unit: "m2", PackAmount: 1, PriceMinor: 2000, Stock: 50}, {SKU: "linoleum", Material: "flooring", OmaCategory: "Линолеум", Unit: "m2", PackAmount: 1, PriceMinor: 500, Stock: 50}}
	for goal, sku := range map[string]string{"Покрасить стены в ванной": "paint", "Плитка в ванной": "tile", "Ламинат на пол": "laminate", "Уложить линолеум": "linoleum"} {
		p := RecommendationProducts(goal, products)
		if len(p) != 1 || p[0].SKU != sku {
			t.Fatalf("%s: %+v", goal, p)
		}
	}
}

package soberi

import (
	"oma/internal/integrations/oma"
	"sort"
	"strings"
)

func RecommendationProducts(goal string, products []oma.Product) []oma.Product {
	goal = strings.ToLower(strings.Split(goal, "\nУчитываем:")[0])
	kind := "paint"
	if strings.Contains(goal, "ламинат") || strings.Contains(goal, "пол ") || strings.Contains(goal, "пола") || strings.Contains(goal, "полы") || strings.Contains(goal, "линолеум") || strings.HasSuffix(goal, "пол") {
		kind = "flooring"
	}
	if strings.Contains(goal, "краск") || strings.Contains(goal, "покрас") {
		kind = "paint"
	}
	if strings.Contains(goal, "плитк") || strings.Contains(goal, "ванн") && !strings.Contains(goal, "краск") && !strings.Contains(goal, "покрас") {
		kind = "tiling"
	}
	rows := []oma.Product{}
	for _, p := range products {
		p = oma.NormalizeProduct(p)
		if kind == "flooring" && p.OmaCategory != "" {
			if strings.Contains(goal, "ламинат") && p.OmaCategory != "Ламинат" {
				continue
			}
			if strings.Contains(goal, "линолеум") && p.OmaCategory != "Линолеум" {
				continue
			}
		}
		if p.CalculationSupported && p.Material == kind {
			rows = append(rows, p)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		return float64(a.PriceMinor)/a.PackAmount < float64(b.PriceMinor)/b.PackAmount
	})
	// Diverse brands and sizes; a short context keeps GLM usage bounded.
	out := []oma.Product{}
	brands := map[string]int{}
	for _, p := range rows {
		if brands[p.Brand] >= 6 {
			continue
		}
		out = append(out, p)
		brands[p.Brand]++
		if len(out) == 24 {
			break
		}
	}
	return out
}

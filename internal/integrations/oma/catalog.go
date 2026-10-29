package oma

import (
	"regexp"
	"strconv"
	"strings"
)

var volumePattern = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(кг|kg|л|l)(?:\b|[^а-яa-z]|$)`)

// NormalizeProduct conservatively maps scraped sales units. It never treats mass as litres.
func NormalizeProduct(p Product) Product {
	p.CalculationSupported = false
	switch p.Unit {
	case "кг":
		p.Unit = "kg"
	case "л":
		p.Unit = "l"
	}
	p.SalesUnit = "уп."
	if p.UnitsRaw != "" {
		p.SalesUnit = p.UnitsRaw
	}
	raw := strings.ToLower(p.UnitsRaw)
	if raw == "шт" {
		p.SalesUnit = "шт."
	}
	if raw == "кв. м" {
		p.SalesUnit = "м²"
		p.Unit = "m2"
		p.PackAmount = 1
	}
	title := strings.ToLower(p.Title)
	text := strings.ToLower(p.Title + " " + p.Description)
	switch p.Material {
	case "paint":
		if p.OmaCategory != "" && p.OmaCategory != "Краски" {
			break
		}
		blocked := false
		for _, word := range []string{"эмаль", "фасад", "fassaden", "радиатор", "цокол", "ржавч", "металл", "грунт"} {
			if strings.Contains(title, word) {
				blocked = true
			}
		}
		if blocked {
			break
		}
		if p.Unit != "l" && p.Unit != "kg" {
			if m := volumePattern.FindStringSubmatch(title); len(m) > 0 {
				n, e := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
				if e == nil {
					p.PackAmount = n
					if m[2] == "кг" || m[2] == "kg" {
						p.Unit = "kg"
					} else {
						p.Unit = "l"
					}
				}
			}
		}
		if p.OmaCategory != "" && !strings.Contains(text, "стен") && !strings.Contains(text, "интерьер") && !strings.Contains(text, "помещен") && !strings.Contains(text, "внутрен") {
			break
		}
		p.CalculationSupported = (p.Unit == "l" || p.Unit == "kg") && p.Coverage > 0
	case "flooring":
		p.CalculationSupported = p.Unit == "m2" && (p.OmaCategory == "" || p.OmaCategory == "Ламинат" || p.OmaCategory == "Линолеум")
	case "tiling":
		p.CalculationSupported = p.Unit == "m2"
	}
	p.CalculationSupported = p.CalculationSupported && p.Stock > 0 && p.PackAmount > 0 && p.PriceMinor > 0
	if p.OmaCategory != "" && p.CalculationSupported {
		p.CalculationNote = "Предварительный расчёт: единицы продажи взяты из каталога. Расход и упаковку уточните перед покупкой."
	}
	return p
}

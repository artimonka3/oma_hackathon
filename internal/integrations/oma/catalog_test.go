package oma

import "testing"

func TestCatalogCalculationUnits(t *testing.T) {
	base := Product{Material: "paint", Title: "Краска для стен 15 кг", OmaCategory: "Краски", Unit: "кг", PackAmount: 15, Coverage: .1, Stock: 50, PriceMinor: 1000, UnitsRaw: "ведро"}
	p := NormalizeProduct(base)
	if !p.CalculationSupported || p.Unit != "kg" || p.PackAmount != 15 || p.SalesUnit != "ведро" {
		t.Fatalf("mass paint must stay mass: %+v", p)
	}
	base.Title = "Краска для радиаторов 15 кг"
	if NormalizeProduct(base).CalculationSupported {
		t.Fatal("radiator paint cannot be recommended for walls")
	}
	base.Title = "Эмаль для стен"
	base.OmaCategory = "Эмали"
	if NormalizeProduct(base).CalculationSupported {
		t.Fatal("enamel cannot be a wall-paint recommendation")
	}
	for _, category := range []string{"Ковры", "Ковровые дорожки"} {
		p = NormalizeProduct(Product{Material: "flooring", OmaCategory: category, Unit: "m2", PackAmount: 2, Stock: 50, PriceMinor: 1000})
		if p.CalculationSupported {
			t.Fatal("loose carpets cannot be area-based floor replacement")
		}
	}
	p = NormalizeProduct(Product{Material: "tiling", UnitsRaw: "кв. м", Unit: "m2", PackAmount: .36, Stock: 100, PriceMinor: 1500})
	if !p.CalculationSupported || p.PackAmount != 1 || p.SalesUnit != "м²" {
		t.Fatalf("price per m² is not a box price: %+v", p)
	}
}
func TestVolumeFromTitle(t *testing.T) {
	p := NormalizeProduct(Product{Material: "paint", Title: "Краска интерьерная для стен 2,5 л", OmaCategory: "Краски", Unit: "pcs", PackAmount: 1, Coverage: .1, Stock: 10, PriceMinor: 100})
	if !p.CalculationSupported || p.Unit != "l" || p.PackAmount != 2.5 {
		t.Fatalf("bad volume: %+v", p)
	}
}

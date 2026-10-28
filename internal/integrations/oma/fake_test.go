package oma

import (
	"context"
	"errors"
	"testing"

	"oma/internal/apperr"
)

func products() []Product {
	return []Product{
		{SKU: "PAINT-MATT-WHITE", Title: "Краска белая", PriceMinor: 18500, Currency: "BYN", PackAmount: 10, Stock: 10, Demo: true},
		{SKU: "MASKING-TAPE", Title: "Скотч", PriceMinor: 600, Currency: "BYN", Stock: 50, Demo: true},
	}
}

func TestFakeSyncCartIdempotent(t *testing.T) {
	f := NewFake(products())
	req := SyncRequest{
		ProjectID: "prj_demo",
		Lines:     []SyncLine{{LinkID: "prj_demo:PAINT-MATT-WHITE", SKU: "PAINT-MATT-WHITE", Packs: 3}},
	}
	first, err := f.SyncCart(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.SyncCart(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.CartID != second.CartID {
		t.Fatal("повторная синхронизация не должна создавать вторую корзину")
	}
	if second.CartVersion != first.CartVersion+1 {
		t.Fatalf("версия корзины должна расти: %d -> %d", first.CartVersion, second.CartVersion)
	}
	cart, err := f.GetCart("prj_demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Lines) != 1 {
		t.Fatalf("проектные строки удвоились: %+v", cart.Lines)
	}
	if cart.TotalMinor != 3*18500 {
		t.Fatalf("итог корзины = %d, ожидался %d", cart.TotalMinor, 3*18500)
	}
}

func TestFakeSyncCartUnknownSKU(t *testing.T) {
	f := NewFake(products())
	_, err := f.SyncCart(context.Background(), SyncRequest{
		ProjectID: "prj_demo",
		Lines:     []SyncLine{{LinkID: "l", SKU: "UNKNOWN", Packs: 1}},
	})
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != "INVALID_INPUT" {
		t.Fatalf("ожидался INVALID_INPUT для неизвестного артикула: %v", err)
	}
}

func TestFakeSearch(t *testing.T) {
	f := NewFake(products())
	found, err := f.SearchProducts(context.Background(), "краск")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].SKU != "PAINT-MATT-WHITE" {
		t.Fatalf("поиск работает некорректно: %+v", found)
	}
	if _, err := f.GetProduct(context.Background(), "UNKNOWN"); err == nil {
		t.Fatal("ожидалась ошибка для отсутствующего артикула")
	}
}

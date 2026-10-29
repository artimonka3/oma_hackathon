package soberi_test

import (
	"context"
	"oma/fixtures"
	"oma/internal/ai"
	"oma/internal/events"
	"oma/internal/jobs"
	"oma/internal/mockoma"
	"oma/internal/project"
	"oma/internal/soberi"
	"testing"
)

func TestMaterialSelectionInventoryAndRounding(t *testing.T) {
	ctx := context.Background()
	ev := events.NewLog()
	ps := project.NewService(project.NewMemoryRepo(), ev)
	shop := mockoma.New(mockoma.Options{Products: fixtures.Products()})
	svc := soberi.NewService(ps, mockoma.Adapter{Service: shop}, jobs.NewQueue(), ai.NewStub(), ev)
	owner := project.UserID("owner")
	p, e := ps.Create(ctx, owner, project.CreateInput{Goal: "Покрасить стены"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.CalculateSpecification(ctx, owner, string(p.ID)); e == nil {
		t.Fatal("calculation without dimensions must fail")
	}
	if _, e = ps.AddRoom(ctx, owner, p.ID, project.AddRoomInput{Name: "Room", WidthMM: 5000, LengthMM: 10000, HeightMM: 2700, DimensionsConfirmed: true}); e != nil {
		t.Fatal(e)
	}
	job, e := svc.StartDesignJob(ctx, owner, string(p.ID))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.HandleDesignJob(ctx, job); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.SelectDesign(ctx, owner, string(p.ID), "variant_PAINT-MATT-WHITE"); e != nil {
		t.Fatal(e)
	}
	sp, e := svc.CalculateSpecification(ctx, owner, string(p.ID))
	if e != nil {
		t.Fatal(e)
	}
	if sp.NewPurchase[0].SKU != "PAINT-MATT-WHITE" || sp.NewPurchase[0].Packs != 2 {
		t.Fatalf("wrong selected material: %+v", sp)
	}
	// Purchase history is not proof of usable stock.
	_, e = ps.SuggestInventory(ctx, owner, p.ID, []project.HistorySuggestion{{SKU: "PAINT-MATT-WHITE", Amount: "100", Unit: "l"}})
	if e != nil {
		t.Fatal(e)
	}
	again, e := svc.CalculateSpecification(ctx, owner, string(p.ID))
	if e != nil {
		t.Fatal(e)
	}
	if again.TotalMinor != sp.TotalMinor {
		t.Fatal("suggested lot reduced purchase")
	}
	if _, e = ps.AddInventory(ctx, owner, p.ID, project.InventoryInput{SKU: "PAINT-MATT-WHITE", Amount: "10", Unit: "l"}); e != nil {
		t.Fatal(e)
	}
	sp, e = svc.CalculateSpecification(ctx, owner, string(p.ID))
	if e != nil {
		t.Fatal(e)
	}
	if sp.NewPurchase[0].Packs != 1 {
		t.Fatalf("confirmed stock not subtracted: %+v", sp)
	}
	if _, e = svc.CartSync(ctx, owner, string(p.ID)); e != nil {
		t.Fatal(e)
	}
	// A different finish must replace the linked cart lines, not append both colours.
	if _, e = svc.SelectDesign(ctx, owner, string(p.ID), "variant_PAINT-MATT-CREAM"); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.CartSync(ctx, owner, string(p.ID)); e == nil {
		t.Fatal("stale specification synchronized")
	}
	if _, e = svc.CalculateSpecification(ctx, owner, string(p.ID)); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.CartSync(ctx, owner, string(p.ID)); e != nil {
		t.Fatal(e)
	}
	cart, e := shop.CartForProject(string(p.ID))
	if e != nil || len(cart.Lines) != 1 || cart.Lines[0].SKU != "PAINT-MATT-CREAM" {
		t.Fatalf("old cart lines retained: %+v %v", cart, e)
	}
	// 50 m2 + 10% reserve = exactly 25 packs of 2.2 m2, not 26 due to float noise.
	f, e := ps.Create(ctx, owner, project.CreateInput{Goal: "Ламинат на пол"})
	if e != nil {
		t.Fatal(e)
	}
	ps.AddRoom(ctx, owner, f.ID, project.AddRoomInput{Name: "Floor", WidthMM: 5000, LengthMM: 10000, HeightMM: 2700, DimensionsConfirmed: true})
	j, _ := svc.StartDesignJob(ctx, owner, string(f.ID))
	svc.HandleDesignJob(ctx, j)
	svc.SelectDesign(ctx, owner, string(f.ID), "variant_LAMINATE-OAK")
	sp, e = svc.CalculateSpecification(ctx, owner, string(f.ID))
	if e != nil || sp.NewPurchase[0].Packs != 25 {
		t.Fatalf("rounding: %+v %v", sp, e)
	}
}

package ai

import "testing"

func TestNormalizeModelVariants(t *testing.T) {
	d := normalizeDraft(Draft{Variants: []Variant{{ID: "same", SKUs: []string{"a", "unknown", "b"}}, {ID: "same", Title: "Repeated", SKUs: []string{"a"}}}}, PrepareRequest{AllowedSKUs: []string{"a", "b"}, Products: []CatalogItem{{SKU: "a", Title: "Paint"}, {SKU: "b", Title: "Floor"}}})
	if len(d.Variants) != 2 {
		t.Fatalf("%+v", d)
	}
	if e := ValidateDraft(d, []string{"a", "b"}); e != nil {
		t.Fatal(e)
	}
}

// Package ai — заменяемое подключение диалоговой и мультимодальной модели.
//
// Модель возвращает только структурированный черновик; ответ проходит
// валидацию доменными правилами до записи в проект (раздел 8 ТЗ).
// StubProvider используется для минимального запуска без внешней модели.
package ai

import (
	"context"
	"strings"
)

// Variant — один объяснимый вариант дизайна из черновика модели.
type Variant struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	SKUs    []string `json:"skus"`
}

// Draft — структурированный черновик модели.
type Draft struct {
	Variants  []Variant `json:"variants"`
	Questions []string  `json:"questions"`
	Model     string    `json:"model,omitempty"`
	Mode      string    `json:"mode"` // live | demo
}

// PrepareRequest — вход задания подготовки вариантов.
type PrepareRequest struct {
	ProjectID   string          `json:"project_id"`
	Goal        string          `json:"goal"`
	RoomNames   []string        `json:"room_names"`
	AllowedSKUs []string        `json:"allowed_skus"`
	Products    []CatalogItem   `json:"products,omitempty"`
	Rooms       []RoomGeometry  `json:"rooms,omitempty"`
	Inventory   []InventoryFact `json:"inventory,omitempty"`
}

// Provider — контракт внешней модели. Заменяемый интерфейс: конкретный
// провайдер подключается без изменения доменного кода.
type Provider interface {
	PrepareVariants(ctx context.Context, req PrepareRequest) (Draft, error)
}

// StubProvider — заглушка для минимального запуска (AI_MODE=demo).
type StubProvider struct{}

// NewStub возвращает заглушку провайдера.
func NewStub() Provider { return StubProvider{} }

// PrepareVariants возвращает подготовленный демо-черновик с тремя
// объяснимыми вариантами; ответ помечен признаком demo.
func (StubProvider) PrepareVariants(ctx context.Context, req PrepareRequest) (Draft, error) {
	category := "PAINT-"
	if strings.Contains(strings.ToLower(req.Goal), "пол") || strings.Contains(strings.ToLower(req.Goal), "ламинат") {
		category = "LAMINATE-"
	}
	if strings.Contains(strings.ToLower(req.Goal), "плитк") || strings.Contains(strings.ToLower(req.Goal), "ванн") {
		category = "TILE-"
	}
	for _, p := range req.Products {
		if v := map[string]string{"paint": "PAINT-", "flooring": "LAMINATE-", "tiling": "TILE-"}[p.Material]; v != "" {
			category = v
			break
		}
	}
	variants := []Variant{}
	for _, sku := range req.AllowedSKUs {
		matches := strings.HasPrefix(sku, category)
		for _, p := range req.Products {
			if p.SKU == sku && p.Material == map[string]string{"PAINT-": "paint", "LAMINATE-": "flooring", "TILE-": "tiling"}[category] {
				matches = true
			}
		}
		if matches && len(variants) < 4 {
			variants = append(variants, Variant{ID: "variant_" + sku, Title: map[string]string{"PAINT-MATT-WHITE": "Светлые стены", "PAINT-MATT-CREAM": "Тёплый кремовый", "LAMINATE-OAK": "Натуральный дуб", "TILE-STONE": "Светлый камень"}[sku], Summary: "Готовый вариант из каталога: проверьте назначение покрытия и совместимость с основанием.", SKUs: []string{sku}})
		}
	}
	for i := range variants {
		if variants[i].Title == "" {
			variants[i].Title = variants[i].SKUs[0]
			for _, p := range req.Products {
				if p.SKU == variants[i].SKUs[0] {
					variants[i].Title = p.Title
				}
			}
		}
	}
	// Compatibility with existing API demo clients.
	if len(variants) > 0 {
		v := variants[0]
		v.ID = "variant_balance"
		variants = append(variants, v)
	}
	return Draft{Mode: "demo", Variants: variants, Questions: []string{"Подтвердите реальные размеры до покупки"}}, nil
}

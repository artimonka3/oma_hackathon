// Package soberi — домен ОМА.Собери: запрос, варианты, выбор версии,
// спецификация и подготовка связанной корзины (раздел 4 продуктовой
// спецификации). Бизнес-логика живёт в Service.
package soberi

import "time"

// SelectedDesign — подтверждённая пользователем версия дизайна.
type SelectedDesign struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	VariantID string    `json:"variant_id"`
	Title     string    `json:"title"`
	SKUs      []string  `json:"skus"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// RoomNeed — расчётная потребность помещения.
type RoomNeed struct {
	RoomID      string  `json:"room_id"`
	Name        string  `json:"name"`
	WallAreaM2  float64 `json:"wall_area_m2"`
	PaintLiters float64 `json:"paint_liters,omitempty"`
	PaintAmount float64 `json:"paint_amount,omitempty"`
	PaintUnit   string  `json:"paint_unit,omitempty"`
}

// PurchaseLine — агрегированная новая закупка: идентичные артикулы
// собраны вместе с сохранением распределения по помещениям.
type PurchaseLine struct {
	SKU        string   `json:"sku"`
	Title      string   `json:"title"`
	Packs      int      `json:"packs"`
	PriceMinor int64    `json:"price_minor"`
	TotalMinor int64    `json:"total_minor"`
	RoomIDs    []string `json:"room_ids"`
}

// Specification — рассчитанный состав проекта. Расчёт выполняется
// правилами категории, а не свободной генерацией: количество упаковок
// и суммы считает программная логика.
type Specification struct {
	ID             string         `json:"id"`
	ProjectID      string         `json:"project_id"`
	Version        int64          `json:"version"`
	RoomNeeds      []RoomNeed     `json:"room_needs"`
	NewPurchase    []PurchaseLine `json:"new_purchase"`
	TotalMinor     int64          `json:"total_minor"`
	Currency       string         `json:"currency"`
	Questions      []string       `json:"questions"`
	SourceHash     string         `json:"source_hash"`
	UsedInventory  float64        `json:"used_inventory"`
	RequiredPacks  map[string]int `json:"required_packs"`
	RuleSetVersion string         `json:"rule_set_version"`
	Demo           bool           `json:"demo"`
}

// Consumption (правила расчёта v1): краска, 0.1 л на м2 стены, одна упаковка
// рассчитана на PackAmount литров. Правила версионируются.
const (
	ruleSetVersion   = "v1"
	paintLitersPerM2 = 0.1
	paintCategory    = "paint"
)

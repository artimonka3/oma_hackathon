package project

import "time"

// LotStatus — состояние партии имеющегося материала.
//
// suggested не уменьшает новую закупку (история покупок только
// порождает предложение подтвердить), confirmed фиксирует объём.
type LotStatus string

const (
	LotSuggested LotStatus = "suggested"
	LotConfirmed LotStatus = "confirmed"
)

// InventoryLot — партия имеющегося материала.
type InventoryLot struct {
	ID        string    `json:"id"`
	ProjectID ProjectID `json:"project_id"`
	OwnerID   UserID    `json:"owner_id"`
	SKU       string    `json:"sku"`
	Title     string    `json:"title"`
	Amount    string    `json:"amount"`
	Unit      string    `json:"unit"`
	Status    LotStatus `json:"status"`
	Source    string    `json:"source"` // history | manual
	Reserved  string    `json:"reserved"`
	CreatedAt time.Time `json:"created_at"`
}

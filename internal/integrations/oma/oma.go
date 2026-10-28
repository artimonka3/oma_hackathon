// Package oma — адаптер HTTP API магазина ОМА (OmaClient из раздела 1.1
// технической спецификации). Контракт заменяемый: HTTPClient указывает на
// симулятор Mock OMA или на реальные сервисы ОМА, Fake позволяет собрать
// приложение без отдельного процесса магазина и без реального API ОМА.
package oma

import "context"

// Product — товар каталога.
type Product struct {
	SKU        string `json:"sku"`
	Title      string `json:"title"`
	Category   string `json:"category"`
	Unit       string `json:"unit"`
	PackAmount int    `json:"pack_amount"`
	PriceMinor int64  `json:"price_minor"`
	Currency   string `json:"currency"`
	Stock      int    `json:"stock"`
	Demo       bool   `json:"demo"`
}

// HistoryItem — запись истории покупок торговой учётной записи.
type HistoryItem struct {
	SKU         string `json:"sku"`
	Title       string `json:"title"`
	Unit        string `json:"unit"`
	Amount      string `json:"amount"`
	PurchasedAt string `json:"purchased_at"`
}

// SyncLine — проектная строка корзины. LinkID связывает строку с проектом:
// повторная синхронизация обновляет её, а не добавляет заново (раздел 7.4 ТЗ).
type SyncLine struct {
	LinkID string `json:"link_id"`
	SKU    string `json:"sku"`
	Packs  int    `json:"packs"`
	RoomID string `json:"room_id,omitempty"`
}

// SyncRequest — состав проектных строк для связанной корзины.
type SyncRequest struct {
	ProjectID string     `json:"project_id"`
	Lines     []SyncLine `json:"lines"`
}

// SyncResult — результат создания/обновления связанной корзины.
type SyncResult struct {
	CartID      string `json:"cart_id"`
	CartVersion int64  `json:"cart_version"`
	Upserted    int    `json:"upserted"`
	TotalMinor  int64  `json:"total_minor"`
	Currency    string `json:"currency"`
}

// Client — контракт интеграции с магазином ОМА.
type Client interface {
	SearchProducts(ctx context.Context, query string) ([]Product, error)
	GetProduct(ctx context.Context, sku string) (Product, error)
	PurchaseHistory(ctx context.Context, accountID string) ([]HistoryItem, error)
	SyncCart(ctx context.Context, req SyncRequest) (SyncResult, error)
}

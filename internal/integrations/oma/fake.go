package oma

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"oma/internal/apperr"
)

// Fake — встроенный симулятор магазина для минимального запуска:
// приложение работает без реального API ОМА и без отдельного процесса
// Mock OMA. Ключевые интеграционные контракты соблюдаются: синхронизация
// корзины идемпотентна по связям строк проекта.
type Fake struct {
	mu       sync.Mutex
	products []Product
	history  []HistoryItem
	carts    map[string]*fakeCart // project_id -> cart
	cartSeq  int
}

type fakeCart struct {
	id      string
	version int64
	total   int64
	lines   map[string]SyncLine // link_id -> line
}

// NewFake создаёт симулятор на фикстурном каталоге.
func NewFake(products []Product) *Fake {
	return &Fake{
		products: products,
		history: []HistoryItem{
			{SKU: "PAINT-MATT-WHITE", Title: "Краска для стен матовая, белая, 10 л", Unit: "l", Amount: "около 4 л", PurchasedAt: "2026-05-12"},
			{SKU: "MASKING-TAPE", Title: "Малярный скотч, 48 мм", Unit: "roll", Amount: "2 рулона", PurchasedAt: "2026-04-02"},
		},
		carts: map[string]*fakeCart{},
	}
}

func (f *Fake) bySKU(sku string) (Product, bool) {
	for _, p := range f.products {
		if p.SKU == sku {
			return p, true
		}
	}
	return Product{}, false
}

// SearchProducts фильтрует каталог по вхождению подстроки.
func (f *Fake) SearchProducts(ctx context.Context, query string) ([]Product, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Product, 0, len(f.products))
	for _, p := range f.products {
		if q == "" || strings.Contains(strings.ToLower(p.Title), q) || strings.Contains(strings.ToLower(p.Category), q) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out, nil
}

// GetProduct возвращает товар или ошибку интеграции.
func (f *Fake) GetProduct(ctx context.Context, sku string) (Product, error) {
	if p, ok := f.bySKU(sku); ok {
		return p, nil
	}
	return Product{}, apperr.NotFound("артикул не найден в каталоге: " + sku)
}

// PurchaseHistory отдаёт демо-историю покупок.
func (f *Fake) PurchaseHistory(ctx context.Context, accountID string) ([]HistoryItem, error) {
	out := make([]HistoryItem, len(f.history))
	copy(out, f.history)
	return out, nil
}

// CartView — снимок корзины симулятора для проверок и отладки.
type CartView struct {
	ID         string     `json:"id"`
	Version    int64      `json:"version"`
	Lines      []SyncLine `json:"lines"`
	TotalMinor int64      `json:"total_minor"`
}

// GetCart возвращает снимок связанной корзины проекта.
func (f *Fake) GetCart(projectID string) (*CartView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cart, ok := f.carts[projectID]
	if !ok {
		return nil, apperr.NotFound("корзина проекта не найдена")
	}
	out := &CartView{ID: cart.id, Version: cart.version, TotalMinor: cart.total}
	for _, l := range cart.lines {
		out.Lines = append(out.Lines, l)
	}
	return out, nil
}

// SyncCart создаёт или обновляет связанную корзину проекта.
// Повторный вызов обновляет те же строки по LinkID, не удваивая покупку.
func (f *Fake) SyncCart(ctx context.Context, req SyncRequest) (SyncResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if req.ProjectID == "" {
		return SyncResult{}, apperr.InvalidInput("project_id обязателен для связанной корзины", nil)
	}
	var unknown []string
	for _, l := range req.Lines {
		if _, ok := f.bySKU(l.SKU); !ok {
			unknown = append(unknown, l.SKU)
		}
	}
	if len(unknown) > 0 {
		return SyncResult{}, apperr.InvalidInput("неизвестные артикулы каталога", map[string]any{"skus": unknown})
	}
	cart, ok := f.carts[req.ProjectID]
	if !ok {
		f.cartSeq++
		cart = &fakeCart{id: fmt.Sprintf("cart_fake_%02d", f.cartSeq), lines: map[string]SyncLine{}}
		f.carts[req.ProjectID] = cart
	}
	upserted := 0
	var total int64
	for _, l := range req.Lines {
		if l.Packs <= 0 {
			return SyncResult{}, apperr.InvalidInput("количество упаковок должно быть положительным", nil)
		}
		cart.lines[l.LinkID] = l
		upserted++
	}
	for _, l := range cart.lines {
		if p, ok := f.bySKU(l.SKU); ok {
			total += int64(l.Packs) * p.PriceMinor
		}
	}
	cart.version++
	cart.total = total
	return SyncResult{
		CartID:      cart.id,
		CartVersion: cart.version,
		Upserted:    upserted,
		TotalMinor:  total,
		Currency:    "BYN",
	}, nil
}

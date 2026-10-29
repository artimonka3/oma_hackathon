package oma

import (
	"context"
	"time"
)

// CartLine — строка корзины. Проектные строки связаны link_id.
type CartLine struct {
	LinkID      string `json:"link_id"`
	SKU         string `json:"sku"`
	RoomID      string `json:"room_id,omitempty"`
	Packs       int    `json:"packs"`
	ProjectLine bool   `json:"project_line"`
	PriceMinor  int64  `json:"price_minor"`
}

// Cart — торговая корзина с версионированием.
type Cart struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	Lines      []CartLine `json:"lines"`
	Version    int64      `json:"version"`
	TotalMinor int64      `json:"total_minor"`
	Currency   string     `json:"currency"`
}

// Quote — неизменяемая котировка корзины с суммой и сроком.
type Quote struct {
	ID         string     `json:"id"`
	CartID     string     `json:"cart_id"`
	ProjectID  string     `json:"project_id"`
	Lines      []CartLine `json:"lines"`
	TotalMinor int64      `json:"total_minor"`
	Currency   string     `json:"currency"`
	Version    int64      `json:"version"`
	ExpiresAt  time.Time  `json:"expires_at"`
}

// CheckoutSession — оформление котировки.
type CheckoutSession struct {
	ID      string `json:"id"`
	QuoteID string `json:"quote_id"`
	State   string `json:"state"` // draft | authorized | paid | failed
	OrderID string `json:"order_id,omitempty"`
}

// Delivery — поставка по заказу.
type Delivery struct {
	Status  string    `json:"status"`
	Comment string    `json:"comment"`
	At      time.Time `json:"at"`
}

// Order — заказ с распределением поставок.
type Order struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	CartID     string     `json:"cart_id"`
	QuoteID    string     `json:"quote_id"`
	Lines      []CartLine `json:"lines"`
	TotalMinor int64      `json:"total_minor"`
	Currency   string     `json:"currency"`
	Status     string     `json:"status"`
	Deliveries []Delivery `json:"deliveries"`
	CreatedAt  time.Time  `json:"created_at"`
	Demo       bool       `json:"demo"`
}

type Commerce interface {
	CartForProject(string) (*Cart, error)
	GetCart(string) (*Cart, error)
	CreateQuote(string) (*Quote, error)
	CreateCheckout(string) (*CheckoutSession, error)
	Checkout(string) (CheckoutSession, error)
	Quote(string) (Quote, error)
	Pay(context.Context, string) (*Order, error)
	GetOrder(string) (*Order, error)
	DemoOrderEvent(context.Context, string, string, string) (*Order, error)
}

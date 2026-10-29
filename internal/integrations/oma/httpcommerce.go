package oma

import (
	"context"
	"net/http"
	"net/url"
)

func (c *HTTPClient) CartForProject(pid string) (*Cart, error) {
	var out Cart
	e := c.call(context.Background(), http.MethodGet, "/oma-api/v1/projects/"+url.PathEscape(pid)+"/cart", nil, nil, &out)
	return &out, e
}
func (c *HTTPClient) GetCart(id string) (*Cart, error) {
	var out Cart
	e := c.call(context.Background(), http.MethodGet, "/oma-api/v1/carts/"+url.PathEscape(id), nil, nil, &out)
	return &out, e
}
func (c *HTTPClient) CreateQuote(id string) (*Quote, error) {
	var out Quote
	e := c.call(context.Background(), http.MethodPost, "/oma-api/v1/carts/"+url.PathEscape(id)+"/quotes", nil, map[string]any{}, &out)
	return &out, e
}
func (c *HTTPClient) CreateCheckout(id string) (*CheckoutSession, error) {
	var out CheckoutSession
	e := c.call(context.Background(), http.MethodPost, "/oma-api/v1/checkout-sessions", nil, map[string]string{"quote_id": id}, &out)
	return &out, e
}
func (c *HTTPClient) Checkout(id string) (CheckoutSession, error) {
	var out CheckoutSession
	e := c.call(context.Background(), http.MethodGet, "/oma-api/v1/checkout-sessions/"+url.PathEscape(id), nil, nil, &out)
	return out, e
}
func (c *HTTPClient) Quote(id string) (Quote, error) {
	var out Quote
	e := c.call(context.Background(), http.MethodGet, "/oma-api/v1/quotes/"+url.PathEscape(id), nil, nil, &out)
	return out, e
}
func (c *HTTPClient) Pay(ctx context.Context, id string) (*Order, error) {
	var out Order
	e := c.call(ctx, http.MethodPost, "/oma-api/v1/checkout-sessions/"+url.PathEscape(id)+"/pay", nil, map[string]any{}, &out)
	return &out, e
}
func (c *HTTPClient) GetOrder(id string) (*Order, error) {
	var out Order
	e := c.call(context.Background(), http.MethodGet, "/oma-api/v1/orders/"+url.PathEscape(id), nil, nil, &out)
	return &out, e
}
func (c *HTTPClient) DemoOrderEvent(ctx context.Context, id, action, comment string) (*Order, error) {
	var out Order
	e := c.call(ctx, http.MethodPost, "/oma-api/v1/demo/orders/"+url.PathEscape(id)+"/events", nil, map[string]string{"action": action, "comment": comment}, &out)
	return &out, e
}

var _ Commerce = (*HTTPClient)(nil)

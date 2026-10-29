package oma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"oma/internal/apperr"
)

// HTTPClient обращается к API магазина по HTTP. В минимальном запуске
// base URL указывает на симулятор Mock OMA; в промышленной интеграции
// заменяется на реальные сервисы ОМА без изменения контракта.
type HTTPClient struct {
	base *url.URL
	http *http.Client
}

// NewHTTPClient проверяет base URL и создаёт клиент с ограниченным таймаутом.
func NewHTTPClient(baseURL string) (*HTTPClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("некорректный OMA_API_BASE_URL: %q", baseURL)
	}
	return &HTTPClient{base: u, http: &http.Client{Timeout: 5 * time.Second}}, nil
}

func (c *HTTPClient) call(ctx context.Context, method, path string, query url.Values, body, out any) error {
	full := c.base.String() + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, full, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return apperr.IntegrationUnavailable("магазин ОМА недоступен: " + err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details any    `json:"details"`
		}
		if resp.StatusCode < 500 && json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&e) == nil && e.Code != "" {
			return apperr.New(e.Code, e.Message, e.Details)
		}
		return apperr.IntegrationUnavailable(fmt.Sprintf("магазин ОМА вернул статус %d", resp.StatusCode))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SearchProducts ищет товары каталога.
func (c *HTTPClient) SearchProducts(ctx context.Context, query string) ([]Product, error) {
	q := url.Values{}
	if query != "" {
		q.Set("q", query)
	}
	var out struct {
		Products []Product `json:"products"`
	}
	if err := c.call(ctx, http.MethodGet, "/oma-api/v1/products", q, nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Products {
		out.Products[i] = NormalizeProduct(out.Products[i])
	}
	return out.Products, nil
}

// GetProduct возвращает карточку товара.
func (c *HTTPClient) GetProduct(ctx context.Context, sku string) (Product, error) {
	var out Product
	if err := c.call(ctx, http.MethodGet, "/oma-api/v1/products/"+url.PathEscape(sku), nil, nil, &out); err != nil {
		return Product{}, err
	}
	return NormalizeProduct(out), nil
}

// PurchaseHistory возвращает историю покупок торговой учётной записи.
func (c *HTTPClient) PurchaseHistory(ctx context.Context, accountID string) ([]HistoryItem, error) {
	q := url.Values{"account": []string{accountID}}
	var out struct {
		Items []HistoryItem `json:"items"`
	}
	if err := c.call(ctx, http.MethodGet, "/oma-api/v1/purchase-history", q, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// SyncCart создаёт/обновляет связанные строки торговой корзины.
func (c *HTTPClient) SyncCart(ctx context.Context, req SyncRequest) (SyncResult, error) {
	var out SyncResult
	if err := c.call(ctx, http.MethodPost, "/oma-api/v1/carts/sync", nil, req, &out); err != nil {
		return SyncResult{}, err
	}
	return out, nil
}

func (c *HTTPClient) Ready(ctx context.Context) error {
	var out map[string]any
	return c.call(ctx, http.MethodGet, "/healthz", nil, nil, &out)
}

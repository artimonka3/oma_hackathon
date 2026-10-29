// Package mockoma — домен симулятора магазина ОМА (компонент Mock OMA
// из раздела 1.1 технической спецификации). Отдельный процесс с отдельным
// хранилищем торговых данных: каталог, поиск, история покупок, корзины,
// котировки, симуляция оплаты и доставки. Все данные помечены demo.
//
// Магазин общается с основным backend по HTTP через /internal маршруты
// (авторизация оформления, входящие события), поэтому код не зависит
// от его внутренней базы.
package mockoma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"oma/internal/apperr"
	"oma/internal/id"
	"oma/internal/integrations/oma"
)

type CartLine = oma.CartLine
type Cart = oma.Cart
type Quote = oma.Quote
type CheckoutSession = oma.CheckoutSession
type Delivery = oma.Delivery
type Order = oma.Order

// Options — настройки симулятора.
type Options struct {
	Products       []oma.Product
	LegacyProducts []oma.Product
	AppBaseURL     string // основной backend, например http://localhost:8080; пусто — без обратных вызовов
	ServiceSecret  string
}

// Service — домен симулятора магазина.
type Service struct {
	opts Options
	http *http.Client

	mu            sync.Mutex
	products      []oma.Product
	history       []oma.HistoryItem
	carts         map[string]*Cart
	cartByProject map[string]string
	cartSeq       int
	quotes        map[string]*Quote
	checkouts     map[string]*CheckoutSession
	orders        map[string]*Order
	orderSeq      int
}

// New создаёт симулятор с фикстурным каталогом.
func New(opts Options) *Service {
	return &Service{
		opts:     opts,
		http:     &http.Client{Timeout: 5 * time.Second},
		products: opts.Products,
		history: []oma.HistoryItem{
			{SKU: "PAINT-MATT-WHITE", Title: "Краска для стен матовая, белая, 10 л", Unit: "l", Amount: "около 4 л", PurchasedAt: "2026-05-12"},
			{SKU: "MASKING-TAPE", Title: "Малярный скотч, 48 мм", Unit: "roll", Amount: "2 рулона", PurchasedAt: "2026-04-02"},
		},
		carts:         map[string]*Cart{},
		cartByProject: map[string]string{},
		quotes:        map[string]*Quote{},
		checkouts:     map[string]*CheckoutSession{},
		orders:        map[string]*Order{},
	}
}

func (s *Service) product(sku string) (oma.Product, error) {
	for _, p := range s.opts.LegacyProducts {
		if p.SKU == sku {
			return p, nil
		}
	}
	for _, p := range s.products {
		if p.SKU == sku {
			return p, nil
		}
	}
	return oma.Product{}, apperr.NotFound("артикул не найден: " + sku)
}

// Search ищет товары каталога по названию или категории.
func (s *Service) Search(query, category string) []oma.Product {
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]oma.Product, 0, len(s.products))
	for _, p := range s.products {
		matchQ := q == "" || strings.Contains(strings.ToLower(p.Title), q) || strings.Contains(strings.ToLower(p.SKU), q) || strings.Contains(strings.ToLower(p.Category), q)
		matchC := category == "" || strings.EqualFold(p.Category, category)
		if matchQ && matchC {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out
}

// Get возвращает карточку товара.
func (s *Service) Get(sku string) (oma.Product, error) { return s.product(sku) }

// CategoryStat — категория каталога и число товаров в ней.
type CategoryStat struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// Categories возвращает список категорий каталога с количеством товаров.
func (s *Service) Categories() []CategoryStat {
	counts := map[string]int{}
	for _, p := range s.products {
		counts[p.Category]++
	}
	out := make([]CategoryStat, 0, len(counts))
	for c, n := range counts {
		out = append(out, CategoryStat{Category: c, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

// History отдаёт демо-историю покупок торговой учётной записи.
func (s *Service) History(accountID string) []oma.HistoryItem {
	out := make([]oma.HistoryItem, len(s.history))
	copy(out, s.history)
	return out
}

func recalcTotal(lines []CartLine) int64 {
	var total int64
	for _, l := range lines {
		total += int64(l.Packs) * l.PriceMinor
	}
	return total
}

// SyncCart создаёт или обновляет связанную корзину проекта: строки
// обновляются по link_id и не добавляются заново при повторной синхронизации.
func (s *Service) SyncCart(req oma.SyncRequest) (oma.SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(req.ProjectID) == "" {
		return oma.SyncResult{}, apperr.InvalidInput("project_id обязателен для связанной корзины", nil)
	}

	for _, l := range req.Lines {
		if l.Packs <= 0 {
			return oma.SyncResult{}, apperr.InvalidInput("количество упаковок должно быть положительным", nil)
		}
		if _, err := s.product(l.SKU); err != nil {
			return oma.SyncResult{}, err
		}
	}
	cartID, ok := s.cartByProject[req.ProjectID]
	var cart *Cart
	if ok {
		cart = s.carts[cartID]
	} else {
		s.cartSeq++
		cart = &Cart{
			ID:        fmt.Sprintf("cart_demo_%02d", s.cartSeq),
			ProjectID: req.ProjectID,
			Version:   0,
			Currency:  "BYN",
		}
		s.carts[cart.ID] = cart
		s.cartByProject[req.ProjectID] = cart.ID
	}
	kept := []CartLine{}
	for _, line := range cart.Lines {
		if !line.ProjectLine {
			kept = append(kept, line)
		}
	}
	cart.Lines = kept
	for _, l := range req.Lines {
		p, _ := s.product(l.SKU)
		line := CartLine{
			LinkID:      l.LinkID,
			SKU:         l.SKU,
			RoomID:      l.RoomID,
			Packs:       l.Packs,
			ProjectLine: true,
			PriceMinor:  p.PriceMinor,
		}
		replaced := false
		for i := range cart.Lines {
			if cart.Lines[i].LinkID == l.LinkID {
				// Посторонние строки магазина не трогаем: обновляем только проектные.
				if cart.Lines[i].ProjectLine {
					cart.Lines[i] = line
				}
				replaced = true
				break
			}
		}
		if !replaced {
			cart.Lines = append(cart.Lines, line)
		}
	}
	cart.Version++
	cart.TotalMinor = recalcTotal(cart.Lines)
	return oma.SyncResult{
		CartID:      cart.ID,
		CartVersion: cart.Version,
		Upserted:    len(req.Lines),
		TotalMinor:  cart.TotalMinor,
		Currency:    cart.Currency,
	}, nil
}

// GetCart возвращает актуальный состав и версию корзины.
func (s *Service) GetCart(cartID string) (*Cart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cart, ok := s.carts[cartID]
	if !ok {
		return nil, apperr.NotFound("корзина не найдена")
	}
	cp := *cart
	return &cp, nil
}

// CreateQuote замораживает котировку корзины: снимок строк, сумма, срок.
func (s *Service) CreateQuote(cartID string) (*Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cart, ok := s.carts[cartID]
	if !ok {
		return nil, apperr.NotFound("корзина не найдена")
	}
	q := &Quote{
		ID:         id.New("quote"),
		CartID:     cartID,
		ProjectID:  cart.ProjectID,
		Lines:      append([]CartLine(nil), cart.Lines...),
		TotalMinor: cart.TotalMinor,
		Currency:   cart.Currency,
		Version:    cart.Version,
		ExpiresAt:  time.Now().UTC().Add(15 * time.Minute),
	}
	s.quotes[q.ID] = q
	cp := *q
	return &cp, nil
}

// CreateCheckout создаёт оформление для котировки.
func (s *Service) CreateCheckout(quoteID string) (*CheckoutSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	quote, ok := s.quotes[quoteID]
	if !ok {
		return nil, apperr.NotFound("котировка не найдена")
	}
	sess := &CheckoutSession{ID: id.New("chk"), QuoteID: quote.ID, State: "draft"}
	s.checkouts[sess.ID] = sess
	cp := *sess
	return &cp, nil
}

// Pay имитирует оплату: перед списанием вызывается серверная авторизация
// оформления основного backend (POST /internal/v1/oma/checkout-authorizations);
// одной проверки в браузере недостаточно (раздел 4.4 ТЗ).
func (s *Service) Pay(ctx context.Context, sessionID string) (*Order, error) {
	s.mu.Lock()
	sess, ok := s.checkouts[sessionID]
	if !ok {
		s.mu.Unlock()
		return nil, apperr.NotFound("оформление не найдено")
	}
	quote, ok := s.quotes[sess.QuoteID]
	if !ok {
		s.mu.Unlock()
		return nil, apperr.NotFound("котировка оформления не найдена")
	}
	expired := time.Now().UTC().After(quote.ExpiresAt)
	cp := *sess
	qcp := *quote
	s.mu.Unlock()

	if cp.State == "paid" {
		// Повторный вызов с тем же оформлением возвращает тот же результат.
		s.mu.Lock()
		order := s.orders[cp.OrderID]
		s.mu.Unlock()
		if order != nil {
			return order, nil
		}
	}
	if cp.State != "draft" && cp.State != "authorized" {
		return nil, apperr.New("INVALID_TRANSITION", "оформление уже закрыто: "+cp.State, nil)
	}
	if expired {
		s.mu.Lock()
		sess.State = "failed"
		s.mu.Unlock()
		return nil, apperr.New("QUOTE_EXPIRED", "срок котировки истёк, требуется новая", nil)
	}
	if cp.State == "draft" && s.opts.AppBaseURL != "" {
		authorized, err := s.requestAuthorization(ctx, qcp)
		if err != nil {
			return nil, err
		}
		if !authorized {
			s.mu.Lock()
			sess.State = "failed"
			s.mu.Unlock()
			return nil, apperr.New("CHECKOUT_DECLINED", "backend не авторизовал оформление", nil)
		}
	}
	s.mu.Lock()
	if sess.State == "paid" {
		existing := *s.orders[sess.OrderID]
		s.mu.Unlock()
		return &existing, nil
	}
	sess.State = "authorized"
	s.orderSeq++
	order := &Order{
		ID:         fmt.Sprintf("order_demo_%02d", s.orderSeq),
		ProjectID:  qcp.ProjectID,
		CartID:     qcp.CartID,
		QuoteID:    qcp.ID,
		Lines:      append([]CartLine(nil), qcp.Lines...),
		TotalMinor: qcp.TotalMinor,
		Currency:   qcp.Currency,
		Status:     "paid",
		Deliveries: []Delivery{{Status: "processing", At: time.Now().UTC()}},
		CreatedAt:  time.Now().UTC(),
		Demo:       true,
	}
	s.orders[order.ID] = order
	sess.OrderID = order.ID
	sess.State = "paid"
	s.mu.Unlock()
	s.emitEvent(ctx, "order.paid", "order", order.ID, map[string]any{
		"order_id": order.ID, "cart_id": order.CartID, "project_id": order.ProjectID,
		"total_minor": order.TotalMinor,
	})
	cpOrd := *order
	return &cpOrd, nil
}

// GetOrder возвращает заказ и распределение поставок.
func (s *Service) GetOrder(orderID string) (*Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return nil, apperr.NotFound("заказ не найден")
	}
	cp := *order
	return &cp, nil
}

// DemoOrderEvent управляет доставкой демо-заказа: оплата проходит
// только через оформление.
func (s *Service) DemoOrderEvent(ctx context.Context, orderID, action, comment string) (*Order, error) {
	s.mu.Lock()
	order, ok := s.orders[orderID]
	if !ok {
		s.mu.Unlock()
		return nil, apperr.NotFound("заказ не найден")
	}
	cp := *order
	s.mu.Unlock()
	var status string
	switch action {
	case "deliver":
		status = "delivered"
	case "deliver_partial":
		status = "partially_delivered"
	case "cancel":
		status = "cancelled"
	default:
		return nil, apperr.InvalidInput("action должен быть deliver, deliver_partial или cancel", nil)
	}
	s.mu.Lock()
	order.Deliveries = append(order.Deliveries, Delivery{Status: status, Comment: comment, At: time.Now().UTC()})
	order.Status = status
	s.mu.Unlock()
	s.emitEvent(ctx, "order.delivery_status", "order", orderID, map[string]any{
		"order_id": orderID, "status": status, "project_id": cp.ProjectID,
	})
	out, _ := s.GetOrder(orderID)
	return out, nil
}

// requestAuthorization обращается к серверной точке авторизации backend.
func (s *Service) requestAuthorization(ctx context.Context, quote Quote) (bool, error) {
	if s.opts.AppBaseURL == "" {
		// Без обратных вызовов (изолированный запуск симулятора)
		// авторизация считается полученной.
		return true, nil
	}
	body := map[string]any{
		"quote_id":     quote.ID,
		"cart_id":      quote.CartID,
		"project_id":   quote.ProjectID,
		"total_minor":  quote.TotalMinor,
		"currency":     quote.Currency,
		"cart_version": quote.Version,
	}
	buf, _ := json.Marshal(body)
	url := strings.TrimSuffix(s.opts.AppBaseURL, "/") + "/internal/v1/oma/checkout-authorizations"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.opts.ServiceSecret != "" {
		req.Header.Set("X-Service-Secret", s.opts.ServiceSecret)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return false, apperr.IntegrationUnavailable("основной backend недоступен: " + err.Error())
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// emitEvent отправляет входящее событие магазина в основной backend;
// повторная доставка обрабатывается один раз по идентификатору события.
func (s *Service) emitEvent(ctx context.Context, typ, aggregate, aggregateID string, data map[string]any) {
	payload := map[string]any{
		"event_id":  id.New("omaevt"),
		"type":      typ,
		"aggregate": aggregate,
		"id":        aggregateID,
		"version":   1,
		"data":      data,
	}
	log.Printf("mockoma: событие %s для %s (project_id=%v)", typ, aggregateID, data["project_id"])
	if s.opts.AppBaseURL == "" {
		return
	}
	buf, _ := json.Marshal(payload)
	url := strings.TrimSuffix(s.opts.AppBaseURL, "/") + "/internal/v1/oma/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		log.Printf("mockoma: не удалось создать запрос события: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if s.opts.ServiceSecret != "" {
		req.Header.Set("X-Service-Secret", s.opts.ServiceSecret)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		log.Printf("mockoma: событие %s не доставлено (будет повторено сверкой): %v", typ, err)
		return
	}
	resp.Body.Close()
}

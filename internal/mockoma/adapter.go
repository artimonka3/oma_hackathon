package mockoma

import (
	"context"
	"oma/internal/apperr"
	"oma/internal/integrations/oma"
)

// Adapter reuses the full shop simulator inside the single-process MVP.
type Adapter struct{ Service *Service }

func (a Adapter) SearchProducts(ctx context.Context, q string) ([]oma.Product, error) {
	return a.Service.Search(q, ""), nil
}
func (a Adapter) GetProduct(ctx context.Context, sku string) (oma.Product, error) {
	return a.Service.Get(sku)
}
func (a Adapter) PurchaseHistory(ctx context.Context, account string) ([]oma.HistoryItem, error) {
	return a.Service.History(account), nil
}
func (a Adapter) SyncCart(ctx context.Context, in oma.SyncRequest) (oma.SyncResult, error) {
	return a.Service.SyncCart(in)
}
func (s *Service) CartForProject(pid string) (*Cart, error) {
	s.mu.Lock()
	id := s.cartByProject[pid]
	s.mu.Unlock()
	if id == "" {
		return nil, apperr.NotFound("Корзина ещё не создана")
	}
	return s.GetCart(id)
}
func (s *Service) Checkout(id string) (CheckoutSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.checkouts[id]
	if !ok {
		return CheckoutSession{}, apperr.NotFound("Оформление не найдено")
	}
	return *v, nil
}
func (s *Service) Quote(id string) (Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.quotes[id]
	if !ok {
		return Quote{}, apperr.NotFound("Котировка не найдена")
	}
	return *v, nil
}
